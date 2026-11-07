package ubl_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

var noteKinds = []struct {
	kind     catalog.DocumentType
	root     string
	line     string
	quantity string
	total    string
}{
	{catalog.CreditNote, "CreditNote", "CreditNoteLine", "CreditedQuantity", "LegalMonetaryTotal"},
	{catalog.DebitNote, "DebitNote", "DebitNoteLine", "DebitedQuantity", "RequestedMonetaryTotal"},
}

func sampleNote(kind catalog.DocumentType) model.Note {
	inv := sampleInvoice()
	return model.Note{
		Type: kind, Series: "FC01", Number: 3, IssueDate: inv.IssueDate, Currency: inv.Currency,
		AffectedType: catalog.Invoice, AffectedNumber: "F001-45", Reason: "01", Description: "AJUSTE & <MOTIVO>",
		Company: inv.Company, Client: inv.Client, TaxedAmount: inv.TaxedAmount,
		IGV: inv.IGV, TotalTaxes: inv.TotalTaxes, Payable: inv.Payable,
		Lines: inv.Lines, Legends: inv.Legends,
	}
}

func assertNoteValue(t *testing.T, root node, want string, path ...string) {
	t.Helper()
	if got, ok := root.find(path...); !ok || got != want {
		t.Errorf("%s = %q (present %v), want %q", strings.Join(path, "/"), got, ok, want)
	}
}

func TestNoteValues(t *testing.T) {
	for _, tc := range noteKinds {
		for _, affected := range []catalog.DocumentType{catalog.Invoice, catalog.Receipt} {
			t.Run(tc.root+"/"+string(affected), func(t *testing.T) {
				n := sampleNote(tc.kind)
				n.AffectedType = affected
				if affected == catalog.Receipt {
					n.Series, n.AffectedNumber = "BC01", "B001-45"
				}
				before := sampleNote(tc.kind)
				before.Series, before.AffectedType, before.AffectedNumber = n.Series, n.AffectedType, n.AffectedNumber
				out, err := ubl.Note(n)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(n, before) {
					t.Error("rendering mutated the caller's note")
				}
				root := parse(t, out)
				if root.XMLName.Local != tc.root || root.XMLName.Space != "urn:oasis:names:specification:ubl:schema:xsd:"+tc.root+"-2" {
					t.Fatalf("root = %v", root.XMLName)
				}
				for _, v := range []struct {
					want string
					path []string
				}{
					{"2.1", []string{"UBLVersionID"}},
					{"2.0", []string{"CustomizationID"}},
					{n.Series + "-3", []string{"ID"}},
					{"2026-10-27", []string{"IssueDate"}},
					{"13:05:00", []string{"IssueTime"}},
					{"PEN", []string{"DocumentCurrencyCode"}},
					{n.AffectedNumber, []string{"DiscrepancyResponse", "ReferenceID"}},
					{"01", []string{"DiscrepancyResponse", "ResponseCode"}},
					{n.Description, []string{"DiscrepancyResponse", "Description"}},
					{n.AffectedNumber, []string{"BillingReference", "InvoiceDocumentReference", "ID"}},
					{string(affected), []string{"BillingReference", "InvoiceDocumentReference", "DocumentTypeCode"}},
					{"#" + ubl.SignatureID, []string{"Signature", "DigitalSignatureAttachment", "ExternalReference", "URI"}},
					{n.Company.RUC, []string{"AccountingSupplierParty", "Party", "PartyIdentification", "ID"}},
					{n.Company.LegalName, []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"}},
					{n.Client.Name, []string{"AccountingCustomerParty", "Party", "PartyLegalEntity", "RegistrationName"}},
					{n.Legends[0].Value, []string{"Note"}},
					{"36.00", []string{"TaxTotal", "TaxAmount"}},
					{"200.00", []string{"TaxTotal", "TaxSubtotal", "TaxableAmount"}},
					{"236.00", []string{tc.total, "PayableAmount"}},
					{"1", []string{tc.line, "ID"}},
					{"2", []string{tc.line, tc.quantity}},
					{"200.00", []string{tc.line, "LineExtensionAmount"}},
					{"PRODUCTO <UNO>", []string{tc.line, "Item", "Description"}},
					{"118.00", []string{tc.line, "PricingReference", "AlternativeConditionPrice", "PriceAmount"}},
					{"01", []string{tc.line, "PricingReference", "AlternativeConditionPrice", "PriceTypeCode"}},
					{"100.00", []string{tc.line, "Price", "PriceAmount"}},
					{"10", []string{tc.line, "TaxTotal", "TaxSubtotal", "TaxCategory", "TaxExemptionReasonCode"}},
					{"1000", []string{tc.line, "TaxTotal", "TaxSubtotal", "TaxCategory", "TaxScheme", "ID"}},
				} {
					assertNoteValue(t, root, v.want, v.path...)
				}
				for _, fragment := range []string{
					"<ext:ExtensionContent/>",
					"<cbc:" + tc.quantity + ` unitCode="NIU">2</cbc:` + tc.quantity + ">",
					`<cbc:PayableAmount currencyID="PEN">236.00</cbc:PayableAmount>`,
				} {
					if !bytes.Contains(out, []byte(fragment)) {
						t.Errorf("missing %s", fragment)
					}
				}
				for _, absent := range []string{"PaymentTerms", "ChargeTotalAmount", "PayableRoundingAmount", "InvoiceLine"} {
					if bytes.Contains(out, []byte("<cac:"+absent)) || bytes.Contains(out, []byte("<cbc:"+absent)) {
						t.Errorf("unexpected default %s", absent)
					}
				}
			})
		}
	}
}

func TestNoteValidation(t *testing.T) {
	for _, kind := range noteKinds {
		for _, tc := range []struct {
			name string
			edit func(*model.Note)
			want string
		}{
			{"type", func(n *model.Note) { n.Type = catalog.Invoice }, `ubl: note: type must be "07" or "08", got "01"`},
			{"series", func(n *model.Note) { n.Series = "" }, "ubl: note: series is required"},
			{"zero number", func(n *model.Note) { n.Number = 0 }, "ubl: note: number must be positive"},
			{"negative number", func(n *model.Note) { n.Number = -1 }, "ubl: note: number must be positive"},
			{"currency", func(n *model.Note) { n.Currency = "" }, "ubl: note: currency is required"},
			{"affected type", func(n *model.Note) { n.AffectedType = catalog.CreditNote }, `ubl: note: affected document type must be "01" or "03", got "07"`},
			{"affected number", func(n *model.Note) { n.AffectedNumber = "F00145" }, `ubl: note: affected document number "F00145" must look like F001-123`},
			{"empty affected number", func(n *model.Note) { n.AffectedNumber = "" }, `ubl: note: affected document number "" must look like F001-123`},
			{"description", func(n *model.Note) { n.Description = " \t\n" }, "ubl: note: description is required"},
			{"empty description", func(n *model.Note) { n.Description = "" }, "ubl: note: description is required"},
			{"lines", func(n *model.Note) { n.Lines = nil }, "ubl: document has no lines"},
			{"affectation", func(n *model.Note) { n.Lines[0].Affectation = "99" }, `ubl: line 1: catalog: unsupported IGV affectation "99"`},
			{"second line", func(n *model.Note) {
				n.Lines = append(n.Lines, n.Lines[0])
				n.Lines[1].Affectation = "99"
			}, `ubl: line 2: catalog: unsupported IGV affectation "99"`},
		} {
			t.Run(kind.root+"/"+tc.name, func(t *testing.T) {
				n, before := sampleNote(kind.kind), sampleNote(kind.kind)
				tc.edit(&n)
				tc.edit(&before)
				out, err := ubl.Note(n)
				if err == nil || err.Error() != tc.want || out != nil {
					t.Errorf("Note() = (%q, %v), want (nil, %q)", out, err, tc.want)
				}
				if !reflect.DeepEqual(n, before) {
					t.Error("rejected note was mutated")
				}
			})
		}
	}
}

func TestNoteReasons(t *testing.T) {
	for _, tc := range []struct {
		kind    catalog.DocumentType
		name    string
		valid   []string
		invalid []string
	}{
		{catalog.CreditNote, "credit", []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11", "12", "13"}, []string{"", "00", "14", "99", "1"}},
		{catalog.DebitNote, "debit", []string{"01", "02", "03", "10", "11", "12"}, []string{"", "00", "04", "09", "13", "99", "1"}},
	} {
		for _, reason := range append(append([]string(nil), tc.valid...), tc.invalid...) {
			t.Run(tc.name+"/"+reason, func(t *testing.T) {
				n := sampleNote(tc.kind)
				n.Reason = reason
				out, err := ubl.Note(n)
				valid := false
				for _, r := range tc.valid {
					valid = valid || r == reason
				}
				if valid {
					if err != nil {
						t.Fatal(err)
					}
					assertNoteValue(t, parse(t, out), reason, "DiscrepancyResponse", "ResponseCode")
					return
				}
				cat := "09"
				if tc.kind == catalog.DebitNote {
					cat = "10"
				}
				want := "ubl: " + tc.name + " note: unknown reason " + `"` + reason + `" (catalog ` + cat + ")"
				if err == nil || err.Error() != want || out != nil {
					t.Errorf("Note() = (%q, %v), want (nil, %q)", out, err, want)
				}
			})
		}
	}
}

func TestNoteOptionalValues(t *testing.T) {
	for _, tc := range noteKinds {
		t.Run(tc.root, func(t *testing.T) {
			n := sampleNote(tc.kind)
			n.ChargeTotal, n.Rounding = d("1.25"), d("-0.01")
			n.OrderReference = "ORDER & <1>"
			n.DespatchRefs = []model.DocumentRef{{Type: "09", Number: "T001-9"}}
			n.RelatedDocs = []model.DocumentRef{{Type: "99", Number: "REF & 1"}}
			n.PaymentTerms = &model.PaymentTerms{Credit: true, Amount: d("236"), Installments: []model.Installment{
				{Amount: d("236"), DueDate: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)},
			}}
			n.Lines[0].UnitValue, n.Lines[0].Quantity = d("84.745762711864"), d("1.12345678904")
			n.Lines = append(n.Lines, n.Lines[0])
			n.Lines[1].Affectation = catalog.TaxedFree
			out, err := ubl.Note(n)
			if err != nil {
				t.Fatal(err)
			}
			root := parse(t, out)
			assertNoteValue(t, root, "1.25", tc.total, "ChargeTotalAmount")
			assertNoteValue(t, root, "-0.01", tc.total, "PayableRoundingAmount")
			assertNoteValue(t, root, n.OrderReference, "OrderReference", "ID")
			assertNoteValue(t, root, "T001-9", "DespatchDocumentReference", "ID")
			assertNoteValue(t, root, "09", "DespatchDocumentReference", "DocumentTypeCode")
			assertNoteValue(t, root, "REF & 1", "AdditionalDocumentReference", "ID")
			assertNoteValue(t, root, "99", "AdditionalDocumentReference", "DocumentTypeCode")
			assertNoteValue(t, root, "Credito", "PaymentTerms", "PaymentMeansID")
			assertNoteValue(t, root, "236.00", "PaymentTerms", "Amount")
			var lines []node
			for _, child := range root.Nodes {
				if child.XMLName.Local == tc.line {
					lines = append(lines, child)
				}
			}
			if len(lines) != 2 {
				t.Fatalf("line count = %d, want 2", len(lines))
			}
			for i, line := range lines {
				assertNoteValue(t, line, []string{"1", "2"}[i], "ID")
				assertNoteValue(t, line, "1.123456789", tc.quantity)
				assertNoteValue(t, line, "84.7457627119", "Price", "PriceAmount")
				assertNoteValue(t, line, []string{"01", "02"}[i], "PricingReference", "AlternativeConditionPrice", "PriceTypeCode")
			}
			for _, fragment := range []string{"<cbc:PaymentMeansID>Cuota001</cbc:PaymentMeansID>", "<cbc:PaymentDueDate>2026-11-27</cbc:PaymentDueDate>"} {
				if !bytes.Contains(out, []byte(fragment)) {
					t.Errorf("missing %s", fragment)
				}
			}
		})
	}
}
