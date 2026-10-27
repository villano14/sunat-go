package ubl_test

import (
	"bytes"
	"encoding/xml"
	"flag"
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

var update = flag.Bool("update", false, "rewrite golden files")

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func sampleInvoice() model.Invoice {
	return model.Invoice{
		Type:          catalog.Invoice,
		OperationType: catalog.InternalSale,
		Series:        "F001",
		Number:        1,
		IssueDate:     time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Currency:      "PEN",
		Company: model.Company{
			RUC:       "20000000001",
			LegalName: "EMPRESA DE PRUEBA S.A.C.",
			TradeName: "PRUEBA",
			Address: model.Address{
				Ubigeo:     "150101",
				Department: "LIMA",
				Province:   "LIMA",
				District:   "LIMA",
				Line:       "AV. LOS OLIVOS 123",
			},
		},
		Client: model.Client{
			IdentityType: catalog.RUC,
			Number:       "20123456789",
			Name:         "CLIENTE & ASOCIADOS S.A.",
		},
		TaxedAmount:   d("200"),
		IGV:           d("36"),
		TotalTaxes:    d("36"),
		LineExtension: d("200"),
		TaxInclusive:  d("236"),
		Payable:       d("236"),
		Lines: []model.InvoiceLine{{
			ProductCode: "P001",
			Unit:        "NIU",
			Description: "PRODUCTO <UNO>",
			Quantity:    d("2"),
			UnitValue:   d("100"),
			UnitPrice:   d("118"),
			Affectation: catalog.TaxedOnerous,
			TaxBase:     d("200"),
			IGVPercent:  d("18"),
			IGV:         d("36"),
			TotalTaxes:  d("36"),
			LineAmount:  d("200"),
		}},
		Legends: []model.Legend{{Code: catalog.AmountInWords, Value: "SON DOSCIENTOS TREINTA Y SEIS CON 00/100 SOLES"}},
	}
}

// node is a minimal generic XML tree used to assert values by path.
type node struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Nodes   []node     `xml:",any"`
}

// find returns the text of the first element matching the local-name path.
func (n node) find(path ...string) (string, bool) {
	if len(path) == 0 {
		return strings.TrimSpace(n.Text), true
	}
	for _, c := range n.Nodes {
		if c.XMLName.Local == path[0] {
			if v, ok := c.find(path[1:]...); ok {
				return v, true
			}
		}
	}
	return "", false
}

func parse(t *testing.T, b []byte) node {
	t.Helper()
	var root node
	if err := xml.Unmarshal(b, &root); err != nil {
		t.Fatalf("output is not well-formed XML: %v\n%s", err, b)
	}
	return root
}

func TestInvoiceValues(t *testing.T) {
	out, err := ubl.Invoice(sampleInvoice())
	if err != nil {
		t.Fatalf("Invoice() error: %v", err)
	}
	root := parse(t, out)
	if root.XMLName.Local != "Invoice" || root.XMLName.Space != "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" {
		t.Fatalf("root = %v", root.XMLName)
	}

	want := map[string][]string{
		"2.1":                      {"UBLVersionID"},
		"2.0":                      {"CustomizationID"},
		"F001-1":                   {"ID"},
		"2026-10-27":               {"IssueDate"},
		"13:05:00":                 {"IssueTime"},
		"01":                       {"InvoiceTypeCode"},
		"PEN":                      {"DocumentCurrencyCode"},
		"20000000001":              {"AccountingSupplierParty", "Party", "PartyIdentification", "ID"},
		"EMPRESA DE PRUEBA S.A.C.": {"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"},
		"150101":                   {"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationAddress", "ID"},
		"0000":                     {"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationAddress", "AddressTypeCode"},
		"CLIENTE & ASOCIADOS S.A.": {"AccountingCustomerParty", "Party", "PartyLegalEntity", "RegistrationName"},
		"Contado":                  {"PaymentTerms", "PaymentMeansID"},
		"36.00":                    {"TaxTotal", "TaxAmount"},
		"236.00":                   {"LegalMonetaryTotal", "PayableAmount"},
		"PRODUCTO <UNO>":           {"InvoiceLine", "Item", "Description"},
		"118.00":                   {"InvoiceLine", "PricingReference", "AlternativeConditionPrice", "PriceAmount"},
		"100.00":                   {"InvoiceLine", "Price", "PriceAmount"},
		"10":                       {"InvoiceLine", "TaxTotal", "TaxSubtotal", "TaxCategory", "TaxExemptionReasonCode"},
		"1000":                     {"InvoiceLine", "TaxTotal", "TaxSubtotal", "TaxCategory", "TaxScheme", "ID"},
	}
	for value, path := range want {
		got, ok := root.find(path...)
		if !ok {
			t.Errorf("missing %s", strings.Join(path, "/"))
			continue
		}
		if got != value {
			t.Errorf("%s = %q, want %q", strings.Join(path, "/"), got, value)
		}
	}
	if !bytes.Contains(out, []byte("<ext:ExtensionContent")) {
		t.Error("missing ext:ExtensionContent placeholder for the signature")
	}
}

func TestInvoiceGolden(t *testing.T) {
	out, err := ubl.Invoice(sampleInvoice())
	if err != nil {
		t.Fatalf("Invoice() error: %v", err)
	}
	golden := filepath.Join("testdata", "invoice-basic.xml")
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

func TestInvoiceCredit(t *testing.T) {
	inv := sampleInvoice()
	inv.PaymentTerms = &model.PaymentTerms{
		Credit: true,
		Amount: d("236"),
		Installments: []model.Installment{
			{Amount: d("236"), DueDate: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)},
		},
	}
	out, err := ubl.Invoice(inv)
	if err != nil {
		t.Fatalf("Invoice() error: %v", err)
	}
	parse(t, out)
	for _, s := range []string{
		"<cbc:PaymentMeansID>Credito</cbc:PaymentMeansID>",
		"<cbc:PaymentMeansID>Cuota001</cbc:PaymentMeansID>",
		"<cbc:PaymentDueDate>2026-11-27</cbc:PaymentDueDate>",
	} {
		if !bytes.Contains(out, []byte(s)) {
			t.Errorf("missing %s", s)
		}
	}
}

func TestInvoiceRejectsUnknownAffectation(t *testing.T) {
	inv := sampleInvoice()
	inv.Lines[0].Affectation = "99"
	if _, err := ubl.Invoice(inv); err == nil {
		t.Fatal("expected error for unknown affectation")
	}
}

func TestInvoicePriceDecimals(t *testing.T) {
	inv := sampleInvoice()
	inv.Lines[0].UnitValue = d("84.745762711864")
	out, err := ubl.Invoice(inv)
	if err != nil {
		t.Fatalf("Invoice() error: %v", err)
	}
	got, _ := parse(t, out).find("InvoiceLine", "Price", "PriceAmount")
	if got != "84.7457627119" {
		t.Errorf("Price/PriceAmount = %q, want %q", got, "84.7457627119")
	}
}
