package ubl_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

var batchPET = time.FixedZone("PET", -5*3600)

func batchCompany() model.Company {
	return model.Company{RUC: "20000000001", LegalName: "EMPRESA & PRUEBA S.A.C."}
}

func batchSampleSummary() model.Summary {
	return model.Summary{
		Company:        batchCompany(),
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, batchPET),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, batchPET),
		Correlative:    1,
		Currency:       "PEN",
		Lines: []model.SummaryLine{
			{
				Type: catalog.Receipt, Series: "B001", Number: 1,
				ClientIdentityType: catalog.DNI, ClientNumber: "12345678",
				Status: catalog.SummaryAdd,
				Total:  d("119.30"), TaxedAmount: d("100"), OtherCharges: d("1"),
				IGV: d("18"), ICBPER: d("0.30"), IGVPercent: d("18"),
			},
			{
				Type: catalog.CreditNote, Series: "BC01", Number: 7,
				ClientIdentityType: catalog.IdentityNone, ClientNumber: "-",
				Reference: &model.SummaryReference{Type: catalog.Receipt, Series: "B001", Number: 1},
				Status:    catalog.SummaryAdd,
				Total:     d("50"), ExoneratedAmount: d("50"),
			},
		},
	}
}

func batchSampleVoided() model.Voided {
	return model.Voided{
		Company:        batchCompany(),
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, batchPET),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, batchPET),
		Correlative:    2,
		Lines: []model.VoidedLine{
			{Type: catalog.Invoice, Series: "F001", Number: 12, Reason: "ERROR EN <DATOS> & MONTO"},
			{Type: catalog.CreditNote, Series: "FC01", Number: 3, Reason: "ANULACION"},
		},
	}
}

// batchTexts returns the text of every element matching the local-name path.
func batchTexts(n node, path ...string) []string {
	if len(path) == 0 {
		return []string{strings.TrimSpace(n.Text)}
	}
	var out []string
	for _, c := range n.Nodes {
		if c.XMLName.Local == path[0] {
			out = append(out, batchTexts(c, path[1:]...)...)
		}
	}
	return out
}

// batchChildren returns the child elements matching a local name.
func batchChildren(n node, name string) []node {
	var out []node
	for _, c := range n.Nodes {
		if c.XMLName.Local == name {
			out = append(out, c)
		}
	}
	return out
}

func batchGolden(t *testing.T, name string, out []byte) {
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

func TestSummaryGolden(t *testing.T) {
	out, err := ubl.Summary(batchSampleSummary())
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}
	batchGolden(t, "summary-basic.xml", out)
}

func TestVoidedGolden(t *testing.T) {
	out, err := ubl.Voided(batchSampleVoided())
	if err != nil {
		t.Fatalf("Voided() error: %v", err)
	}
	batchGolden(t, "voided-basic.xml", out)
}

func TestSummaryValues(t *testing.T) {
	out, err := ubl.Summary(batchSampleSummary())
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}
	root := parse(t, out)
	if root.XMLName.Local != "SummaryDocuments" || root.XMLName.Space != "urn:sunat:names:specification:ubl:peru:schema:xsd:SummaryDocuments-1" {
		t.Fatalf("root = %v", root.XMLName)
	}
	for _, p := range []struct {
		want string
		path []string
	}{
		{"2.0", []string{"UBLVersionID"}},
		{"1.1", []string{"CustomizationID"}},
		{"RC-20261028-1", []string{"ID"}},
		{"2026-10-27", []string{"ReferenceDate"}},
		{"2026-10-28", []string{"IssueDate"}},
		{"SIGN20000000001", []string{"Signature", "ID"}},
		{"#SUNATGO-SIGN", []string{"Signature", "DigitalSignatureAttachment", "ExternalReference", "URI"}},
		{"EMPRESA & PRUEBA S.A.C.", []string{"Signature", "SignatoryParty", "PartyName", "Name"}},
		{"20000000001", []string{"AccountingSupplierParty", "CustomerAssignedAccountID"}},
		{"6", []string{"AccountingSupplierParty", "AdditionalAccountID"}},
		{"EMPRESA & PRUEBA S.A.C.", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"}},
	} {
		got := batchTexts(root, p.path...)
		if len(got) != 1 || got[0] != p.want {
			t.Errorf("%s = %q, want %q", strings.Join(p.path, "/"), got, p.want)
		}
	}

	lines := batchChildren(root, "SummaryDocumentsLine")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	l1, l2 := lines[0], lines[1]
	for _, p := range []struct {
		line node
		want []string
		path []string
	}{
		{l1, []string{"1"}, []string{"LineID"}},
		{l1, []string{"03"}, []string{"DocumentTypeCode"}},
		{l1, []string{"B001-1"}, []string{"ID"}},
		{l1, []string{"12345678"}, []string{"AccountingCustomerParty", "CustomerAssignedAccountID"}},
		{l1, []string{"1"}, []string{"AccountingCustomerParty", "AdditionalAccountID"}},
		{l1, []string{"1"}, []string{"Status", "ConditionCode"}},
		{l1, []string{"119.30"}, []string{"TotalAmount"}},
		{l1, []string{"100.00"}, []string{"BillingPayment", "PaidAmount"}},
		{l1, []string{"01"}, []string{"BillingPayment", "InstructionID"}},
		{l1, []string{"true"}, []string{"AllowanceCharge", "ChargeIndicator"}},
		{l1, []string{"1.00"}, []string{"AllowanceCharge", "Amount"}},
		{l1, []string{"18.00", "0.30"}, []string{"TaxTotal", "TaxAmount"}},
		{l1, []string{"18.00", "0.30"}, []string{"TaxTotal", "TaxSubtotal", "TaxAmount"}},
		{l1, []string{"18.00"}, []string{"TaxTotal", "TaxSubtotal", "TaxCategory", "Percent"}},
		{l1, []string{"1000", "7152"}, []string{"TaxTotal", "TaxSubtotal", "TaxCategory", "TaxScheme", "ID"}},
		{l1, []string{"VAT", "OTH"}, []string{"TaxTotal", "TaxSubtotal", "TaxCategory", "TaxScheme", "TaxTypeCode"}},
		{l2, []string{"2"}, []string{"LineID"}},
		{l2, []string{"07"}, []string{"DocumentTypeCode"}},
		{l2, []string{"BC01-7"}, []string{"ID"}},
		{l2, []string{"-"}, []string{"AccountingCustomerParty", "CustomerAssignedAccountID"}},
		{l2, []string{"B001-1"}, []string{"BillingReference", "InvoiceDocumentReference", "ID"}},
		{l2, []string{"03"}, []string{"BillingReference", "InvoiceDocumentReference", "DocumentTypeCode"}},
		{l2, []string{"50.00"}, []string{"BillingPayment", "PaidAmount"}},
		{l2, []string{"02"}, []string{"BillingPayment", "InstructionID"}},
		{l2, []string{"0.00"}, []string{"TaxTotal", "TaxAmount"}},
		{l2, nil, []string{"AllowanceCharge"}},
		{l2, nil, []string{"TaxTotal", "TaxSubtotal", "TaxCategory", "Percent"}},
	} {
		if got := batchTexts(p.line, p.path...); !reflect.DeepEqual(got, p.want) {
			t.Errorf("line %s %s = %q, want %q", batchTexts(p.line, "LineID"), strings.Join(p.path, "/"), got, p.want)
		}
	}
	for _, c := range batchChildren(l1, "TotalAmount") {
		if len(c.Attrs) != 1 || c.Attrs[0].Name.Local != "currencyID" || c.Attrs[0].Value != "PEN" {
			t.Errorf("TotalAmount attrs = %v", c.Attrs)
		}
	}
	if !bytes.Contains(out, []byte("<ext:ExtensionContent/>")) {
		t.Error("missing ext:ExtensionContent placeholder for the signature")
	}
}

func TestSummaryElementOrder(t *testing.T) {
	s := batchSampleSummary()
	l := &s.Lines[0]
	l.ExoneratedAmount, l.UnaffectedAmount = d("1"), d("2")
	l.ExportAmount, l.FreeAmount = d("3"), d("4")
	l.ISC, l.OtherTaxes = d("5"), d("6")
	out, err := ubl.Summary(s)
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}
	line := batchChildren(parse(t, out), "SummaryDocumentsLine")[0]
	var names []string
	for _, c := range line.Nodes {
		names = append(names, c.XMLName.Local)
	}
	want := []string{
		"LineID", "DocumentTypeCode", "ID", "AccountingCustomerParty", "Status", "TotalAmount",
		"BillingPayment", "BillingPayment", "BillingPayment", "BillingPayment", "BillingPayment",
		"AllowanceCharge", "TaxTotal", "TaxTotal", "TaxTotal", "TaxTotal",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("line children = %v, want %v", names, want)
	}
	if got := batchTexts(line, "BillingPayment", "InstructionID"); !reflect.DeepEqual(got, []string{"01", "02", "03", "04", "05"}) {
		t.Errorf("instruction ids = %v", got)
	}
	if got := batchTexts(line, "TaxTotal", "TaxSubtotal", "TaxCategory", "TaxScheme", "ID"); !reflect.DeepEqual(got, []string{"1000", "2000", "9999", "7152"}) {
		t.Errorf("tax scheme ids = %v", got)
	}
}

func TestSummaryIVAPReplacesIGV(t *testing.T) {
	s := batchSampleSummary()
	s.Lines = s.Lines[:1]
	s.Lines[0].IVAP, s.Lines[0].ICBPER = d("4.50"), decimal.Zero
	out, err := ubl.Summary(s)
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}
	line := batchChildren(parse(t, out), "SummaryDocumentsLine")[0]
	if got := batchTexts(line, "TaxTotal", "TaxSubtotal", "TaxCategory", "TaxScheme", "Name"); !reflect.DeepEqual(got, []string{"IVAP"}) {
		t.Errorf("tax names = %v, want only IVAP", got)
	}
	if got := batchTexts(line, "TaxTotal", "TaxAmount"); !reflect.DeepEqual(got, []string{"4.50"}) {
		t.Errorf("tax amounts = %v", got)
	}
	if got := batchTexts(line, "TaxTotal", "TaxSubtotal", "TaxCategory", "Percent"); got != nil {
		t.Errorf("IVAP must not report a percent, got %v", got)
	}
}

func TestSummaryDefaultsToTaxedPayment(t *testing.T) {
	s := batchSampleSummary()
	s.Currency = ""
	s.Lines = s.Lines[:1]
	s.Lines[0].TaxedAmount = decimal.Zero
	out, err := ubl.Summary(s)
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}
	line := batchChildren(parse(t, out), "SummaryDocumentsLine")[0]
	if got := batchTexts(line, "BillingPayment", "PaidAmount"); !reflect.DeepEqual(got, []string{"0.00"}) {
		t.Errorf("payments = %v, want a single 0.00 taxed payment", got)
	}
	if !bytes.Contains(out, []byte(`currencyID="PEN"`)) {
		t.Error("currency must default to PEN")
	}
}

func TestSummaryEscapesText(t *testing.T) {
	s := batchSampleSummary()
	s.Company.LegalName = `A&B <"C">`
	out, err := ubl.Summary(s)
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}
	if got := batchTexts(parse(t, out), "AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"); !reflect.DeepEqual(got, []string{`A&B <"C">`}) {
		t.Errorf("RegistrationName = %q", got)
	}
}

func TestVoidedValues(t *testing.T) {
	out, err := ubl.Voided(batchSampleVoided())
	if err != nil {
		t.Fatalf("Voided() error: %v", err)
	}
	root := parse(t, out)
	if root.XMLName.Local != "VoidedDocuments" || root.XMLName.Space != "urn:sunat:names:specification:ubl:peru:schema:xsd:VoidedDocuments-1" {
		t.Fatalf("root = %v", root.XMLName)
	}
	for _, p := range []struct {
		want string
		path []string
	}{
		{"2.0", []string{"UBLVersionID"}},
		{"1.0", []string{"CustomizationID"}},
		{"RA-20261028-2", []string{"ID"}},
		{"2026-10-27", []string{"ReferenceDate"}},
		{"2026-10-28", []string{"IssueDate"}},
		{"SIGN20000000001", []string{"Signature", "ID"}},
		{"#SUNATGO-SIGN", []string{"Signature", "DigitalSignatureAttachment", "ExternalReference", "URI"}},
		{"20000000001", []string{"AccountingSupplierParty", "CustomerAssignedAccountID"}},
	} {
		got := batchTexts(root, p.path...)
		if len(got) != 1 || got[0] != p.want {
			t.Errorf("%s = %q, want %q", strings.Join(p.path, "/"), got, p.want)
		}
	}
	lines := batchChildren(root, "VoidedDocumentsLine")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for i, want := range []struct{ id, typ, series, number, reason string }{
		{"1", "01", "F001", "12", "ERROR EN <DATOS> & MONTO"},
		{"2", "07", "FC01", "3", "ANULACION"},
	} {
		l := lines[i]
		got := []string{
			batchTexts(l, "LineID")[0], batchTexts(l, "DocumentTypeCode")[0], batchTexts(l, "DocumentSerialID")[0],
			batchTexts(l, "DocumentNumberID")[0], batchTexts(l, "VoidReasonDescription")[0],
		}
		if exp := []string{want.id, want.typ, want.series, want.number, want.reason}; !reflect.DeepEqual(got, exp) {
			t.Errorf("line %d = %q, want %q", i+1, got, exp)
		}
	}
}

func TestBatchIDAndFileName(t *testing.T) {
	s := batchSampleSummary()
	if s.ID() != "RC-20261028-1" || s.FileName() != "20000000001-RC-20261028-1" {
		t.Errorf("summary ID/FileName = %q / %q", s.ID(), s.FileName())
	}
	v := batchSampleVoided()
	if v.ID() != "RA-20261028-2" || v.FileName() != "20000000001-RA-20261028-2" {
		t.Errorf("voided ID/FileName = %q / %q", v.ID(), v.FileName())
	}
}

func TestBatchValidation(t *testing.T) {
	for name, mutate := range map[string]func(*model.Summary){
		"no lines":           func(s *model.Summary) { s.Lines = nil },
		"zero correlative":   func(s *model.Summary) { s.Correlative = 0 },
		"huge correlative":   func(s *model.Summary) { s.Correlative = 100000 },
		"no issue date":      func(s *model.Summary) { s.IssueDate = time.Time{} },
		"no generation date": func(s *model.Summary) { s.GenerationDate = time.Time{} },
		"bad status":         func(s *model.Summary) { s.Lines[0].Status = "9" },
		"no series":          func(s *model.Summary) { s.Lines[0].Series = "" },
	} {
		t.Run("summary "+name, func(t *testing.T) {
			s := batchSampleSummary()
			mutate(&s)
			if _, err := ubl.Summary(s); err == nil || !strings.HasPrefix(err.Error(), "ubl: ") {
				t.Fatalf("error = %v, want ubl error", err)
			}
		})
	}
	for name, mutate := range map[string]func(*model.Voided){
		"no lines":         func(v *model.Voided) { v.Lines = nil },
		"zero correlative": func(v *model.Voided) { v.Correlative = 0 },
		"no reason":        func(v *model.Voided) { v.Lines[0].Reason = "" },
		"no number":        func(v *model.Voided) { v.Lines[0].Number = 0 },
	} {
		t.Run("voided "+name, func(t *testing.T) {
			v := batchSampleVoided()
			mutate(&v)
			if _, err := ubl.Voided(v); err == nil || !strings.HasPrefix(err.Error(), "ubl: ") {
				t.Fatalf("error = %v, want ubl error", err)
			}
		})
	}
}
