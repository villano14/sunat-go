package ubl

import (
	"fmt"
	"strings"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// Note renders a Nota de crédito (07) or Nota de débito (08) as unsigned
// UBL 2.1 XML.
func Note(n model.Note) ([]byte, error) {
	var tmpl string
	switch n.Type {
	case catalog.CreditNote:
		tmpl = "creditnote.xml.tmpl"
		if !catalog.CreditNoteReason(n.Reason).Valid() {
			return nil, fmt.Errorf("ubl: credit note: unknown reason %q (catalog 09)", n.Reason)
		}
	case catalog.DebitNote:
		tmpl = "debitnote.xml.tmpl"
		if !catalog.DebitNoteReason(n.Reason).Valid() {
			return nil, fmt.Errorf("ubl: debit note: unknown reason %q (catalog 10)", n.Reason)
		}
	default:
		return nil, fmt.Errorf("ubl: note: type must be %q or %q, got %q", catalog.CreditNote, catalog.DebitNote, n.Type)
	}
	if err := checkSale("note", n.Series, n.Number, n.Currency); err != nil {
		return nil, err
	}
	if n.AffectedType != catalog.Invoice && n.AffectedType != catalog.Receipt {
		return nil, fmt.Errorf("ubl: note: affected document type must be %q or %q, got %q", catalog.Invoice, catalog.Receipt, n.AffectedType)
	}
	if !strings.Contains(n.AffectedNumber, "-") {
		return nil, fmt.Errorf("ubl: note: affected document number %q must look like F001-123", n.AffectedNumber)
	}
	if strings.TrimSpace(n.Description) == "" {
		return nil, fmt.Errorf("ubl: note: description is required")
	}

	v := saleView{
		SignatureID:    SignatureID,
		ID:             n.ID(),
		IssueDate:      n.IssueDate,
		Currency:       n.Currency,
		Legends:        n.Legends,
		Company:        n.Company,
		Client:         n.Client,
		PaymentTerms:   n.PaymentTerms,
		OrderReference: n.OrderReference,
		DespatchRefs:   n.DespatchRefs,
		RelatedDocs:    n.RelatedDocs,
		TotalTaxes:     n.TotalTaxes,
		ChargeTotal:    n.ChargeTotal,
		Rounding:       n.Rounding,
		Payable:        n.Payable,
		Discrepancy:    &discrepancyView{Reference: n.AffectedNumber, Code: n.Reason, Description: n.Description},
		Billing:        &billingView{Number: n.AffectedNumber, Type: string(n.AffectedType)},
	}
	v.Subtotals = saleTotals{
		taxed: n.TaxedAmount, exonerated: n.ExoneratedAmount, unaffected: n.UnaffectedAmount,
		export: n.ExportAmount, free: n.FreeAmount, freeIGV: n.FreeIGV, igv: n.IGV,
		isc: n.ISC, iscBase: n.ISCBase, ivap: n.IVAP, ivapBase: n.IVAPBase,
		other: n.OtherTaxes, otherBase: n.OtherTaxesBase, icbper: n.ICBPER,
	}.subtotals()
	if err := v.addLines(n.Lines); err != nil {
		return nil, err
	}
	return render(tmpl, v)
}
