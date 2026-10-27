package ubl

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

type invoiceView struct {
	model.Invoice
	SignatureID string
	Subtotals   []subtotalView
	Lines       []lineView
}

type subtotalView struct {
	Taxable decimal.Decimal
	Tax     decimal.Decimal
	Scheme  catalog.TaxScheme
}

type lineView struct {
	model.InvoiceLine
	Scheme catalog.TaxScheme
}

// Invoice renders a Factura (01) or Boleta (03) as unsigned UBL 2.1 XML.
//
// When PaymentTerms is nil a Factura is rendered as "Contado".
func Invoice(inv model.Invoice) ([]byte, error) {
	v := invoiceView{Invoice: inv, SignatureID: SignatureID}
	if v.OperationType == "" {
		v.OperationType = catalog.InternalSale
	}
	if v.PaymentTerms == nil && v.Type == catalog.Invoice {
		v.PaymentTerms = &model.PaymentTerms{}
	}
	if !inv.TaxedAmount.IsZero() || !inv.IGV.IsZero() {
		v.Subtotals = append(v.Subtotals, subtotalView{inv.TaxedAmount, inv.IGV, catalog.SchemeIGV})
	}
	for _, s := range []struct {
		amount decimal.Decimal
		scheme catalog.TaxScheme
	}{
		{inv.ExoneratedAmount, catalog.SchemeExonerated},
		{inv.UnaffectedAmount, catalog.SchemeUnaffected},
		{inv.ExportAmount, catalog.SchemeExport},
		{inv.FreeAmount, catalog.SchemeFree},
	} {
		if !s.amount.IsZero() {
			v.Subtotals = append(v.Subtotals, subtotalView{s.amount, decimal.Zero, s.scheme})
		}
	}
	for i, l := range inv.Lines {
		scheme, err := catalog.SchemeFor(l.Affectation)
		if err != nil {
			return nil, fmt.Errorf("ubl: line %d: %w", i+1, err)
		}
		if l.PriceType == "" {
			l.PriceType = catalog.UnitPriceWithTax
		}
		v.Lines = append(v.Lines, lineView{InvoiceLine: l, Scheme: scheme})
	}
	return render("invoice.xml.tmpl", v)
}
