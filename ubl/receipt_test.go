package ubl_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func TestReceiptValuesAndManualAmounts(t *testing.T) {
	inv := sampleInvoice()
	inv.Type, inv.Series = catalog.Receipt, "B001"
	inv.Client = model.Client{IdentityType: catalog.DNI, Number: "12345678", Name: "CONSUMIDOR & FAMILIA"}
	// Deliberately differ from the lines: rendering must not invoke Totals or
	// reconcile caller amounts. This is not a business-valid issuance fixture.
	inv.TaxedAmount, inv.IGV, inv.TotalTaxes = d("199.91"), d("35.98"), d("35.98")
	inv.LineExtension, inv.TaxInclusive, inv.Payable = d("199.91"), d("235.89"), d("219.91")
	inv.AllowanceTotal, inv.ChargeTotal = d("10"), d("2")
	inv.PrepaidTotal, inv.Rounding = d("7.99"), d("0.01")
	before := inv
	before.Lines = append([]model.InvoiceLine(nil), inv.Lines...)
	out, err := ubl.Invoice(inv)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inv, before) {
		t.Error("rendering mutated the caller's boleta")
	}
	if inv.ID() != "B001-1" || inv.FileName() != "20000000001-03-B001-1" {
		t.Errorf("ID=%q filename=%q", inv.ID(), inv.FileName())
	}
	root := parse(t, out)
	const ns = "urn:oasis:names:specification:ubl:schema:xsd:"
	if root.XMLName.Local != "Invoice" || root.XMLName.Space != ns+"Invoice-2" {
		t.Fatalf("root = %v", root.XMLName)
	}
	for _, v := range []struct {
		want string
		path string
	}{
		{"2.1", "UBLVersionID"}, {"2.0", "CustomizationID"},
		{"B001-1", "ID"}, {"03", "InvoiceTypeCode"}, {"PEN", "DocumentCurrencyCode"},
		{"12345678", "AccountingCustomerParty/Party/PartyIdentification/ID"},
		{"CONSUMIDOR & FAMILIA", "AccountingCustomerParty/Party/PartyLegalEntity/RegistrationName"},
		{"199.91", "TaxTotal/TaxSubtotal/TaxableAmount"},
		{"35.98", "TaxTotal/TaxAmount"}, {"35.98", "TaxTotal/TaxSubtotal/TaxAmount"},
		{"199.91", "LegalMonetaryTotal/LineExtensionAmount"},
		{"235.89", "LegalMonetaryTotal/TaxInclusiveAmount"},
		{"10.00", "LegalMonetaryTotal/AllowanceTotalAmount"},
		{"2.00", "LegalMonetaryTotal/ChargeTotalAmount"},
		{"7.99", "LegalMonetaryTotal/PrepaidAmount"},
		{"0.01", "LegalMonetaryTotal/PayableRoundingAmount"},
		{"219.91", "LegalMonetaryTotal/PayableAmount"},
		{"200.00", "InvoiceLine/LineExtensionAmount"},
		{"36.00", "InvoiceLine/TaxTotal/TaxAmount"},
		{"118.00", "InvoiceLine/PricingReference/AlternativeConditionPrice/PriceAmount"},
		{"100.00", "InvoiceLine/Price/PriceAmount"},
	} {
		got, ok := root.find(strings.Split(v.path, "/")...)
		if !ok || got != v.want {
			t.Errorf("%s = %q (present %v), want %q", v.path, got, ok, v.want)
		}
	}
	for _, fragment := range []string{
		`xmlns:cac="` + ns + `CommonAggregateComponents-2"`,
		`xmlns:cbc="` + ns + `CommonBasicComponents-2"`,
		`xmlns:ext="` + ns + `CommonExtensionComponents-2"`,
		`xmlns:ds="http://www.w3.org/2000/09/xmldsig#"`,
		`<cbc:InvoiceTypeCode listID="0101">03</cbc:InvoiceTypeCode>`,
		`<cbc:ID schemeID="1">12345678</cbc:ID>`,
		`<cbc:PayableAmount currencyID="PEN">219.91</cbc:PayableAmount>`,
		`<ext:ExtensionContent/>`,
	} {
		if !bytes.Contains(out, []byte(fragment)) {
			t.Errorf("missing %s", fragment)
		}
	}
	if bytes.Contains(out, []byte("<ds:Signature")) {
		t.Error("ubl.Invoice must render unsigned XML")
	}
}
