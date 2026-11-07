package ubl

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// saleView is the data shared by the Invoice, CreditNote and DebitNote
// templates. Invoice-only and note-only parts are nil/empty for the others.
type saleView struct {
	SignatureID string

	ID            string
	TypeCode      string // invoice only
	OperationType string // invoice only
	IssueDate     time.Time
	DueDate       *time.Time // invoice only
	Currency      string

	Legends     []model.Legend
	Observation string

	Company  model.Company
	Client   model.Client
	Seller   *model.Client
	Delivery *model.Address

	PaymentTerms *model.PaymentTerms

	OrderReference string
	DespatchRefs   []model.DocumentRef
	RelatedDocs    []model.DocumentRef
	Prepayments    []prepaymentView
	Detraction     *model.Detraction
	Perception     *perceptionView

	AllowanceCharges []model.AllowanceCharge

	TotalTaxes     decimal.Decimal
	Subtotals      []subtotalView
	LineExtension  decimal.Decimal
	TaxInclusive   decimal.Decimal
	AllowanceTotal decimal.Decimal
	ChargeTotal    decimal.Decimal
	PrepaidTotal   decimal.Decimal
	Rounding       decimal.Decimal
	Payable        decimal.Decimal

	Lines []lineView

	// Notes only.
	Discrepancy *discrepancyView
	Billing     *billingView
}

type prepaymentView struct {
	Index   int
	DocType string
	Number  string
	Total   decimal.Decimal
}

type perceptionView struct {
	Regime  catalog.AllowanceChargeReason
	Factor  decimal.Decimal
	Amount  decimal.Decimal
	Base    decimal.Decimal
	Total   decimal.Decimal
	Percent decimal.Decimal
}

type discrepancyView struct {
	Reference   string
	Code        string
	Description string
}

type billingView struct {
	Number string
	Type   string
}

type subtotalView struct {
	Taxable decimal.Decimal
	Tax     decimal.Decimal
	Scheme  catalog.TaxScheme
	NoBase  bool // ICBPER carries no TaxableAmount
}

type lineView struct {
	model.InvoiceLine
	Currency string
	Scheme   catalog.TaxScheme
}

// saleTotals groups the totals shared by model.Invoice and model.Note.
type saleTotals struct {
	taxed, exonerated, unaffected, export, free, freeIGV, igv decimal.Decimal
	isc, iscBase, ivap, ivapBase, other, otherBase, icbper    decimal.Decimal
}

// subtotals lists the document-level tax subtotals in Greenter's order:
// ISC, IGV, INA, EXO, GRA, EXP, IVAP, OTROS, ICBPER.
func (t saleTotals) subtotals() []subtotalView {
	var out []subtotalView
	if !t.isc.IsZero() {
		out = append(out, subtotalView{Taxable: t.iscBase, Tax: t.isc, Scheme: catalog.SchemeISC})
	}
	if !t.taxed.IsZero() || !t.igv.IsZero() {
		out = append(out, subtotalView{Taxable: t.taxed, Tax: t.igv, Scheme: catalog.SchemeIGV})
	}
	for _, s := range []subtotalView{
		{Taxable: t.unaffected, Scheme: catalog.SchemeUnaffected},
		{Taxable: t.exonerated, Scheme: catalog.SchemeExonerated},
		{Taxable: t.free, Tax: t.freeIGV, Scheme: catalog.SchemeFree},
		{Taxable: t.export, Scheme: catalog.SchemeExport},
	} {
		if !s.Taxable.IsZero() {
			out = append(out, s)
		}
	}
	if !t.ivap.IsZero() {
		out = append(out, subtotalView{Taxable: t.ivapBase, Tax: t.ivap, Scheme: catalog.SchemeIVAP})
	}
	if !t.other.IsZero() {
		out = append(out, subtotalView{Taxable: t.otherBase, Tax: t.other, Scheme: catalog.SchemeOthers})
	}
	if !t.icbper.IsZero() {
		out = append(out, subtotalView{Tax: t.icbper, Scheme: catalog.SchemeICBPER, NoBase: true})
	}
	return out
}

func (v *saleView) addLines(lines []model.InvoiceLine) error {
	if len(lines) == 0 {
		return fmt.Errorf("ubl: document has no lines")
	}
	for i, l := range lines {
		scheme, err := catalog.SchemeFor(l.Affectation)
		if err != nil {
			return fmt.Errorf("ubl: line %d: %w", i+1, err)
		}
		if l.PriceType == "" {
			l.PriceType = catalog.UnitPriceWithTax
			if l.Affectation.Free() {
				l.PriceType = catalog.ReferenceValue
			}
		}
		l.AllowanceCharges = orderAllowances(l.AllowanceCharges)
		v.Lines = append(v.Lines, lineView{InvoiceLine: l, Currency: v.Currency, Scheme: scheme})
	}
	return nil
}

// orderAllowances puts charges before discounts, as Greenter does, keeping the
// caller's order inside each group.
func orderAllowances(in []model.AllowanceCharge) []model.AllowanceCharge {
	out := append([]model.AllowanceCharge(nil), in...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Charge && !out[j].Charge })
	return out
}

func checkSale(kind, series string, number int64, currency string) error {
	switch {
	case series == "":
		return fmt.Errorf("ubl: %s: series is required", kind)
	case number < 1:
		return fmt.Errorf("ubl: %s: number must be positive", kind)
	case currency == "":
		return fmt.Errorf("ubl: %s: currency is required", kind)
	}
	return nil
}

// Invoice renders a Factura (01) or Boleta (03) as unsigned UBL 2.1 XML.
//
// When PaymentTerms is nil a Factura is rendered as "Contado".
func Invoice(inv model.Invoice) ([]byte, error) {
	if inv.Type != catalog.Invoice && inv.Type != catalog.Receipt {
		return nil, fmt.Errorf("ubl: invoice: type must be %q or %q, got %q", catalog.Invoice, catalog.Receipt, inv.Type)
	}
	if err := checkSale("invoice", inv.Series, inv.Number, inv.Currency); err != nil {
		return nil, err
	}
	v := saleView{
		SignatureID:    SignatureID,
		ID:             inv.ID(),
		TypeCode:       string(inv.Type),
		OperationType:  string(inv.OperationType),
		IssueDate:      inv.IssueDate,
		Currency:       inv.Currency,
		Legends:        inv.Legends,
		Observation:    inv.Observation,
		Company:        inv.Company,
		Client:         inv.Client,
		DueDate:        inv.DueDate,
		Seller:         inv.Seller,
		Delivery:       inv.Delivery,
		PaymentTerms:   inv.PaymentTerms,
		OrderReference: inv.OrderReference,
		DespatchRefs:   inv.DespatchRefs,
		RelatedDocs:    inv.RelatedDocs,
		Detraction:     inv.Detraction,
		TotalTaxes:     inv.TotalTaxes,
		LineExtension:  inv.LineExtension,
		TaxInclusive:   inv.TaxInclusive,
		AllowanceTotal: inv.AllowanceTotal,
		ChargeTotal:    inv.ChargeTotal,
		PrepaidTotal:   inv.PrepaidTotal,
		Rounding:       inv.Rounding,
		Payable:        inv.Payable,
	}
	if v.OperationType == "" {
		v.OperationType = string(catalog.InternalSale)
	}
	if v.PaymentTerms == nil && inv.Type == catalog.Invoice {
		v.PaymentTerms = &model.PaymentTerms{}
	}
	for i, p := range inv.Prepayments {
		v.Prepayments = append(v.Prepayments, prepaymentView{Index: i + 1, DocType: p.DocType, Number: p.Number, Total: p.Total})
	}
	v.AllowanceCharges = orderAllowances(inv.AllowanceCharges)
	if p := inv.Perception; p != nil {
		v.Perception = &perceptionView{
			Regime: p.Regime, Factor: p.Percent.Div(decimal.NewFromInt(100)), Percent: p.Percent,
			Amount: p.Amount, Base: p.Base, Total: p.Total,
		}
	}
	v.Subtotals = saleTotals{
		taxed: inv.TaxedAmount, exonerated: inv.ExoneratedAmount, unaffected: inv.UnaffectedAmount,
		export: inv.ExportAmount, free: inv.FreeAmount, freeIGV: inv.FreeIGV, igv: inv.IGV,
		isc: inv.ISC, iscBase: inv.ISCBase, ivap: inv.IVAP, ivapBase: inv.IVAPBase,
		other: inv.OtherTaxes, otherBase: inv.OtherTaxesBase, icbper: inv.ICBPER,
	}.subtotals()
	if err := v.addLines(inv.Lines); err != nil {
		return nil, err
	}
	return render("invoice.xml.tmpl", v)
}
