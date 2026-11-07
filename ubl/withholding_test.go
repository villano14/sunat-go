package ubl_test

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func withholdingDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func withholdingDay(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.FixedZone("PET", -5*3600))
}

func withholdingCompany() model.Company {
	return model.Company{
		RUC:       "20000000001",
		LegalName: "EMPRESA DE PRUEBA S.A.C.",
		TradeName: "PRUEBA",
		Address: model.Address{
			Ubigeo: "150101", Department: "LIMA", Province: "LIMA",
			District: "LIMA", Line: "AV. LOS OLIVOS 123",
		},
	}
}

func sampleRetention() model.Retention {
	return model.Retention{
		Series: "R001", Number: 1,
		IssueDate:     time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Company:       withholdingCompany(),
		Supplier:      model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "PROVEEDOR & ASOCIADOS S.A."},
		Regime:        catalog.RetentionRate3,
		Rate:          withholdingDec("3"),
		Note:          "OBSERVACION <1>",
		TotalRetained: withholdingDec("6"),
		TotalPaid:     withholdingDec("194"),
		Details: []model.RetentionDetail{
			{
				DocumentType: catalog.Invoice, Number: "F001-1", IssueDate: withholdingDay(10, 20),
				Total: withholdingDec("100"), Currency: "USD",
				Payments: []model.Payment{
					{Currency: "USD", Amount: withholdingDec("60"), Date: withholdingDay(10, 25)},
					{Currency: "USD", Amount: withholdingDec("40"), Date: withholdingDay(10, 26)},
				},
				RetentionDate: withholdingDay(10, 26), Retained: withholdingDec("6"), NetPayable: withholdingDec("94"),
				Exchange: &model.ExchangeRate{
					SourceCurrency: "USD", TargetCurrency: "PEN",
					Rate: withholdingDec("3.75"), Date: withholdingDay(10, 26),
				},
			},
			{
				DocumentType: catalog.Invoice, Number: "F001-2", IssueDate: withholdingDay(10, 21),
				Total: withholdingDec("100"), Currency: "PEN",
				Payments: []model.Payment{{Currency: "PEN", Amount: withholdingDec("100"), Date: withholdingDay(10, 26)}},
				// No retention block: date and amounts left zero.
			},
		},
	}
}

func samplePerception() model.Perception {
	return model.Perception{
		Series: "P001", Number: 1,
		IssueDate:      time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Company:        withholdingCompany(),
		Customer:       model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE & ASOCIADOS S.A."},
		Regime:         catalog.PerceptionInternalSale,
		Note:           "OBSERVACION <1>",
		TotalPerceived: withholdingDec("2"),
		TotalCollected: withholdingDec("102"),
		Details: []model.PerceptionDetail{
			{
				DocumentType: catalog.Invoice, Number: "F001-1", IssueDate: withholdingDay(10, 20),
				Total: withholdingDec("100"), Currency: "PEN",
				Collections: []model.Payment{
					{Currency: "PEN", Amount: withholdingDec("102"), Date: withholdingDay(10, 25)},
				},
				PerceptionDate: withholdingDay(10, 25), Perceived: withholdingDec("2"), NetCollectable: withholdingDec("102"),
			},
		},
	}
}

// withholdingNode is a minimal generic XML tree used to assert values by path.
type withholdingNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr        `xml:",any,attr"`
	Text    string            `xml:",chardata"`
	Nodes   []withholdingNode `xml:",any"`
}

func (n withholdingNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// findAll returns every element matching the local-name path.
func (n withholdingNode) findAll(path ...string) []withholdingNode {
	if len(path) == 0 {
		return []withholdingNode{n}
	}
	var out []withholdingNode
	for _, c := range n.Nodes {
		if c.XMLName.Local == path[0] {
			out = append(out, c.findAll(path[1:]...)...)
		}
	}
	return out
}

func (n withholdingNode) text(path ...string) (string, bool) {
	nodes := n.findAll(path...)
	if len(nodes) == 0 {
		return "", false
	}
	return strings.TrimSpace(nodes[0].Text), true
}

func withholdingParse(t *testing.T, b []byte) withholdingNode {
	t.Helper()
	var root withholdingNode
	if err := xml.Unmarshal(b, &root); err != nil {
		t.Fatalf("output is not well-formed XML: %v\n%s", err, b)
	}
	return root
}

// withholdingCheck asserts the text of the first element at each "A/B/C" local-name path.
func withholdingCheck(t *testing.T, root withholdingNode, want map[string]string) {
	t.Helper()
	for path, value := range want {
		got, ok := root.text(strings.Split(path, "/")...)
		if !ok {
			t.Errorf("missing %s", path)
			continue
		}
		if got != value {
			t.Errorf("%s = %q, want %q", path, got, value)
		}
	}
}

func withholdingGolden(t *testing.T, name string, out []byte) {
	t.Helper()
	golden := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(golden, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(out, want) {
		t.Errorf("output differs from %s; run go test ./ubl -update after review\n%s", golden, out)
	}
}

func TestRetentionValues(t *testing.T) {
	out, err := ubl.Retention(sampleRetention())
	if err != nil {
		t.Fatalf("Retention() error: %v", err)
	}
	root := withholdingParse(t, out)
	if root.XMLName.Local != "Retention" || root.XMLName.Space != "urn:sunat:names:specification:ubl:peru:schema:xsd:Retention-1" {
		t.Fatalf("root = %v", root.XMLName)
	}
	withholdingCheck(t, root, map[string]string{
		"UBLVersionID":    "2.0",
		"CustomizationID": "1.0",
		"Signature/ID":    "SIGN20000000001",
		"Signature/DigitalSignatureAttachment/ExternalReference/URI": "#" + ubl.SignatureID,
		"ID":                                  "R001-1",
		"IssueDate":                           "2026-10-27",
		"IssueTime":                           "13:05:00",
		"AgentParty/PartyIdentification/ID":   "20000000001",
		"AgentParty/PartyName/Name":           "PRUEBA",
		"AgentParty/PostalAddress/ID":         "150101",
		"AgentParty/PostalAddress/StreetName": "AV. LOS OLIVOS 123",
		"AgentParty/PostalAddress/Country/IdentificationCode": "PE",
		"AgentParty/PartyLegalEntity/RegistrationName":        "EMPRESA DE PRUEBA S.A.C.",
		"ReceiverParty/PartyIdentification/ID":                "20123456789",
		"ReceiverParty/PartyLegalEntity/RegistrationName":     "PROVEEDOR & ASOCIADOS S.A.",
		"SUNATRetentionSystemCode":                            "01",
		"SUNATRetentionPercent":                               "3.00",
		"Note":                                                "OBSERVACION <1>",
		"TotalInvoiceAmount":                                  "6.00",
		"SUNATTotalPaid":                                      "194.00",
	})
	if got := root.findAll("TotalInvoiceAmount")[0].attr("currencyID"); got != "PEN" {
		t.Errorf("TotalInvoiceAmount currencyID = %q, want PEN", got)
	}
	if got := root.findAll("ReceiverParty", "PartyIdentification", "ID")[0].attr("schemeID"); got != "6" {
		t.Errorf("ReceiverParty schemeID = %q, want 6", got)
	}

	refs := root.findAll("SUNATRetentionDocumentReference")
	if len(refs) != 2 {
		t.Fatalf("document references = %d, want 2", len(refs))
	}
	first := refs[0]
	withholdingCheck(t, first, map[string]string{
		"ID":                 "F001-1",
		"IssueDate":          "2026-10-20",
		"TotalInvoiceAmount": "100.00",
		"Payment/ID":         "1",
		"Payment/PaidAmount": "60.00",
		"Payment/PaidDate":   "2026-10-25",
		"SUNATRetentionInformation/SUNATRetentionAmount":            "6.00",
		"SUNATRetentionInformation/SUNATRetentionDate":              "2026-10-26",
		"SUNATRetentionInformation/SUNATNetTotalPaid":               "94.00",
		"SUNATRetentionInformation/ExchangeRate/SourceCurrencyCode": "USD",
		"SUNATRetentionInformation/ExchangeRate/TargetCurrencyCode": "PEN",
		"SUNATRetentionInformation/ExchangeRate/CalculationRate":    "3.750000",
	})
	if got := first.findAll("ID")[0].attr("schemeID"); got != "01" {
		t.Errorf("reference ID schemeID = %q, want 01", got)
	}
	if got := first.findAll("TotalInvoiceAmount")[0].attr("currencyID"); got != "USD" {
		t.Errorf("reference currencyID = %q, want USD", got)
	}
	payments := first.findAll("Payment")
	if len(payments) != 2 {
		t.Fatalf("payments = %d, want 2", len(payments))
	}
	if got, _ := payments[1].text("ID"); got != "2" {
		t.Errorf("second payment ID = %q, want 2", got)
	}
	if len(refs[1].findAll("SUNATRetentionInformation")) != 0 {
		t.Error("detail without retention date and amounts must omit SUNATRetentionInformation")
	}
	if !bytes.Contains(out, []byte("<ext:ExtensionContent")) {
		t.Error("missing ext:ExtensionContent placeholder for the signature")
	}
}

func TestPerceptionValues(t *testing.T) {
	out, err := ubl.Perception(samplePerception())
	if err != nil {
		t.Fatalf("Perception() error: %v", err)
	}
	root := withholdingParse(t, out)
	if root.XMLName.Local != "Perception" || root.XMLName.Space != "urn:sunat:names:specification:ubl:peru:schema:xsd:Perception-1" {
		t.Fatalf("root = %v", root.XMLName)
	}
	withholdingCheck(t, root, map[string]string{
		"UBLVersionID":    "2.0",
		"CustomizationID": "1.0",
		"Signature/ID":    "SIGN20000000001",
		"Signature/DigitalSignatureAttachment/ExternalReference/URI": "#" + ubl.SignatureID,
		"ID":        "P001-1",
		"IssueDate": "2026-10-27",
		"IssueTime": "13:05:00",
		"ReceiverParty/PartyLegalEntity/RegistrationName": "CLIENTE & ASOCIADOS S.A.",
		"SUNATPerceptionSystemCode":                       "01",
		"SUNATPerceptionPercent":                          "2.00",
		"Note":                                            "OBSERVACION <1>",
		"TotalInvoiceAmount":                              "2.00",
		"SUNATTotalCashed":                                "102.00",
	})
	refs := root.findAll("SUNATPerceptionDocumentReference")
	if len(refs) != 1 {
		t.Fatalf("document references = %d, want 1", len(refs))
	}
	withholdingCheck(t, refs[0], map[string]string{
		"ID":                 "F001-1",
		"TotalInvoiceAmount": "100.00",
		"Payment/ID":         "1",
		"Payment/PaidAmount": "102.00",
		"Payment/PaidDate":   "2026-10-25",
		"SUNATPerceptionInformation/SUNATPerceptionAmount": "2.00",
	})
	withholdingCheck(t, refs[0], map[string]string{
		"SUNATPerceptionInformation/SUNATNetTotalCashed": "102.00",
	})
	if len(refs[0].findAll("SUNATPerceptionInformation", "ExchangeRate")) != 0 {
		t.Error("PEN detail without Exchange must omit cac:ExchangeRate")
	}
}

func TestRetentionGolden(t *testing.T) {
	out, err := ubl.Retention(sampleRetention())
	if err != nil {
		t.Fatalf("Retention() error: %v", err)
	}
	withholdingGolden(t, "retention-basic.xml", out)
}

func TestPerceptionGolden(t *testing.T) {
	out, err := ubl.Perception(samplePerception())
	if err != nil {
		t.Fatalf("Perception() error: %v", err)
	}
	withholdingGolden(t, "perception-basic.xml", out)
}

func TestWithholdingDefaultRate(t *testing.T) {
	r := sampleRetention()
	r.Rate = decimal.Zero
	r.Regime = catalog.RetentionRate6
	out, err := ubl.Retention(r)
	if err != nil {
		t.Fatalf("Retention() error: %v", err)
	}
	withholdingCheck(t, withholdingParse(t, out), map[string]string{
		"SUNATRetentionSystemCode": "02",
		"SUNATRetentionPercent":    "6.00",
	})

	for regime, rate := range map[catalog.PerceptionRegime]string{
		catalog.PerceptionInternalSale: "2.00",
		catalog.PerceptionFuel:         "1.00",
		catalog.PerceptionSpecialRate:  "0.50",
	} {
		p := samplePerception()
		p.Regime = regime
		out, err := ubl.Perception(p)
		if err != nil {
			t.Fatalf("Perception(%s) error: %v", regime, err)
		}
		if got, _ := withholdingParse(t, out).text("SUNATPerceptionPercent"); got != rate {
			t.Errorf("regime %s rate = %q, want %q", regime, got, rate)
		}
	}
}

func TestWithholdingExplicitRateWins(t *testing.T) {
	p := samplePerception()
	p.Rate = withholdingDec("1.5")
	out, err := ubl.Perception(p)
	if err != nil {
		t.Fatalf("Perception() error: %v", err)
	}
	if got, _ := withholdingParse(t, out).text("SUNATPerceptionPercent"); got != "1.50" {
		t.Errorf("rate = %q, want 1.50", got)
	}
}

func TestWithholdingRejectsUnknownRegime(t *testing.T) {
	r := sampleRetention()
	r.Regime = "99"
	if _, err := ubl.Retention(r); err == nil {
		t.Error("Retention: expected error for unknown regime")
	}
	p := samplePerception()
	p.Regime = ""
	if _, err := ubl.Perception(p); err == nil {
		t.Error("Perception: expected error for empty regime")
	}
}

func TestWithholdingOptionalNote(t *testing.T) {
	r := sampleRetention()
	r.Note = ""
	out, err := ubl.Retention(r)
	if err != nil {
		t.Fatalf("Retention() error: %v", err)
	}
	if _, ok := withholdingParse(t, out).text("Note"); ok {
		t.Error("empty Note must omit cbc:Note")
	}
}

func TestWithholdingTradeNameFallsBackToLegalName(t *testing.T) {
	p := samplePerception()
	p.Company.TradeName = ""
	out, err := ubl.Perception(p)
	if err != nil {
		t.Fatalf("Perception() error: %v", err)
	}
	if got, _ := withholdingParse(t, out).text("AgentParty", "PartyName", "Name"); got != "EMPRESA DE PRUEBA S.A.C." {
		t.Errorf("AgentParty name = %q, want legal name", got)
	}
}

func TestWithholdingElementOrder(t *testing.T) {
	// SUNAT validates against an xsd:sequence, so order matters.
	for name, tc := range map[string]struct {
		render func() ([]byte, error)
		tags   []string
	}{
		"retention": {
			func() ([]byte, error) { return ubl.Retention(sampleRetention()) },
			[]string{"UBLExtensions", "UBLVersionID", "CustomizationID", "Signature", "ID", "IssueDate", "IssueTime",
				"AgentParty", "ReceiverParty", "SUNATRetentionSystemCode", "SUNATRetentionPercent", "Note",
				"TotalInvoiceAmount", "SUNATTotalPaid", "SUNATRetentionDocumentReference"},
		},
		"perception": {
			func() ([]byte, error) { return ubl.Perception(samplePerception()) },
			[]string{"UBLExtensions", "UBLVersionID", "CustomizationID", "Signature", "ID", "IssueDate", "IssueTime",
				"AgentParty", "ReceiverParty", "SUNATPerceptionSystemCode", "SUNATPerceptionPercent", "Note",
				"TotalInvoiceAmount", "SUNATTotalCashed", "SUNATPerceptionDocumentReference"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := tc.render()
			if err != nil {
				t.Fatal(err)
			}
			root := withholdingParse(t, out)
			var got []string
			for _, c := range root.Nodes {
				got = append(got, c.XMLName.Local)
			}
			// Collapse repeated trailing document references.
			if len(got) > len(tc.tags) {
				got = got[:len(tc.tags)]
			}
			if strings.Join(got, ",") != strings.Join(tc.tags, ",") {
				t.Errorf("child order = %v, want %v", got, tc.tags)
			}
		})
	}
}
