package ubl

import (
	"fmt"
	"regexp"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// purchaseSeries is the format of an electronic Liquidación de compra series.
var purchaseSeries = regexp.MustCompile(`^L[A-Z0-9]{3}$`)

// purchaseMaxNumber is the largest correlative of a comprobante (8 digits).
const purchaseMaxNumber = 99999999

type purchaseView struct {
	model.PurchaseSettlement
	ID              string
	SignatureID     string
	Subtotals       []subtotalView
	IncomeTaxScheme catalog.TaxScheme
	Lines           []purchaseLineView
}

type purchaseLineView struct {
	model.PurchaseSettlementLine
	Scheme catalog.TaxScheme
}

// PurchaseSettlement renders a Liquidación de compra (04) as unsigned UBL 2.1
// SelfBilledInvoice XML, the format SUNAT accepts for this document. Unlike an
// Invoice, the buyer (Company) is the AccountingCustomerParty and comes before
// the seller (AccountingSupplierParty).
//
// The document TaxTotal reports the IGV with subtotals for the taxed (also
// when no other operation is present), exonerated and unaffected amounts, plus
// the retained income tax (scheme 3000), which is not added to TaxAmount. Each
// line reports its IGV subtotal and its income tax subtotal.
//
// OperationType defaults to 0501 and Currency to PEN.
func PurchaseSettlement(p model.PurchaseSettlement) ([]byte, error) {
	if err := checkPurchase(p); err != nil {
		return nil, err
	}
	v := purchaseView{
		PurchaseSettlement: p,
		ID:                 p.ID(),
		SignatureID:        SignatureID,
		IncomeTaxScheme:    catalog.SchemeIncomeTax,
	}
	if v.OperationType == "" {
		v.OperationType = catalog.PurchaseInternal
	}
	if v.Currency == "" {
		v.Currency = "PEN"
	}
	if !p.TaxedAmount.IsZero() || (p.ExoneratedAmount.IsZero() && p.UnaffectedAmount.IsZero()) {
		v.Subtotals = append(v.Subtotals, subtotalView{Taxable: p.TaxedAmount, Tax: p.IGV, Scheme: catalog.SchemeIGV})
	}
	if !p.ExoneratedAmount.IsZero() {
		v.Subtotals = append(v.Subtotals, subtotalView{Taxable: p.ExoneratedAmount, Scheme: catalog.SchemeExonerated})
	}
	if !p.UnaffectedAmount.IsZero() {
		v.Subtotals = append(v.Subtotals, subtotalView{Taxable: p.UnaffectedAmount, Scheme: catalog.SchemeUnaffected})
	}
	v.Subtotals = append(v.Subtotals, subtotalView{Taxable: p.IncomeTaxBase, Tax: p.IncomeTax, Scheme: catalog.SchemeIncomeTax})

	for _, l := range p.Lines {
		scheme, _ := catalog.SchemeFor(l.Affectation) // checked by checkPurchase
		if l.Unit == "" {
			l.Unit = "KGM"
		}
		v.Lines = append(v.Lines, purchaseLineView{PurchaseSettlementLine: l, Scheme: scheme})
	}
	return render("purchase.xml.tmpl", v)
}

func checkPurchase(p model.PurchaseSettlement) error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("ubl: purchase settlement: "+format, args...)
	}
	switch op := p.OperationType; {
	case !purchaseSeries.MatchString(p.Series):
		return fail("series %q must be L followed by 3 uppercase alphanumerics, e.g. L001", p.Series)
	case p.Number < 1 || p.Number > purchaseMaxNumber:
		return fail("number %d out of range 1..%d", p.Number, purchaseMaxNumber)
	case p.IssueDate.IsZero():
		return fail("issue date is required")
	case op != "" && op != catalog.PurchaseInternal && op != catalog.PurchaseAdvance && op != catalog.PurchaseGold:
		return fail("operation type must be 0501, 0502 or 0503, got %q", op)
	case p.Company.RUC == "" || p.Company.LegalName == "":
		return fail("company RUC and legal name are required")
	case p.Seller.IdentityType != catalog.DNI && p.Seller.IdentityType != catalog.ForeignCard:
		return fail("seller identity type must be DNI (1) or carné de extranjería (4), got %q", p.Seller.IdentityType)
	case p.Seller.Number == "" || p.Seller.Name == "":
		return fail("seller number and name are required")
	case p.Seller.Ubigeo == "" || p.Seller.Line == "":
		return fail("seller ubigeo and address line are required")
	case !p.Seller.AddressType.Valid():
		return fail("seller address type must be 01 to 05, got %q", p.Seller.AddressType)
	case !p.Location.Type.Valid():
		return fail("location type must be 01 to 05, got %q", p.Location.Type)
	case p.Location.Ubigeo == "" || p.Location.Line == "":
		return fail("location ubigeo and address line are required")
	case len(p.Lines) == 0:
		return fail("at least one line is required")
	}
	for i, l := range p.Lines {
		switch {
		case l.Affectation != catalog.TaxedOnerous && l.Affectation != catalog.ExoneratedOnerous && l.Affectation != catalog.UnaffectedOnerous:
			return fail("line %d: affectation must be 10, 20 or 30, got %q", i+1, l.Affectation)
		case l.Description == "":
			return fail("line %d: description is required", i+1)
		case l.SunatProductCode == "":
			return fail("line %d: SUNAT product code is required", i+1)
		case !l.Quantity.IsPositive():
			return fail("line %d: quantity must be positive", i+1)
		case l.Affectation != catalog.TaxedOnerous && !l.IGV.IsZero():
			return fail("line %d: only taxed lines (10) carry IGV", i+1)
		}
		for j, a := range l.Attributes {
			if a.Code == "" || a.Name == "" || a.Value == "" {
				return fail("line %d attribute %d: code, name and value are required", i+1, j+1)
			}
		}
	}
	for i, r := range p.DespatchRefs {
		if r.Type != "09" && r.Type != "31" {
			return fail("despatch reference %d: type must be 09 or 31, got %q", i+1, r.Type)
		}
	}
	return nil
}
