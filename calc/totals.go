package calc

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// TotalsInput supplies calculated lines and explicit document adjustments, all
// in one caller-chosen currency. Amounts must already be rounded to cents.
// Totals neither allocates base-changing adjustments nor infers tax policy.
type TotalsInput struct {
	Lines          []model.InvoiceLine
	AllowanceTotal decimal.Decimal // Additional non-base document discounts; excludes line reason 01.
	ChargeTotal    decimal.Decimal // Non-base document charges only.
	PrepaidTotal   decimal.Decimal // Gross prepayments, including their taxes; no base allocation.
	Rounding       decimal.Decimal // Signed payable adjustment; the only negative amount allowed.
}

// TotalsResult contains the monetary fields of model.Invoice. Tax-category
// amounts sum TaxBase; LineExtension sums paid LineAmount. FreeAmount/FreeIGV
// are reference/notional amounts, excluded from TotalTaxes and payable totals.
type TotalsResult struct {
	TaxedAmount      decimal.Decimal
	ExoneratedAmount decimal.Decimal
	UnaffectedAmount decimal.Decimal
	ExportAmount     decimal.Decimal
	FreeAmount       decimal.Decimal
	FreeIGV          decimal.Decimal
	IGV              decimal.Decimal
	ISC              decimal.Decimal
	ISCBase          decimal.Decimal
	IVAP             decimal.Decimal
	IVAPBase         decimal.Decimal
	OtherTaxes       decimal.Decimal
	OtherTaxesBase   decimal.Decimal
	ICBPER           decimal.Decimal
	TotalTaxes       decimal.Decimal
	LineExtension    decimal.Decimal
	TaxInclusive     decimal.Decimal
	AllowanceTotal   decimal.Decimal
	ChargeTotal      decimal.Decimal
	PrepaidTotal     decimal.Decimal
	Rounding         decimal.Decimal
	Payable          decimal.Decimal
}

// Totals sums rounded line amounts without recomputing taxes or rates. Line IGV
// belongs to IVAP for affectation 17, to FreeIGV for free lines, otherwise to IGV.
// Reason 00 discounts are already reflected in LineAmount; reason 01 discounts
// are added to AllowanceTotal. All line charges and other reasons are rejected.
//
// TaxInclusive = LineExtension + TotalTaxes.
// Payable = TaxInclusive - AllowanceTotal + ChargeTotal - PrepaidTotal + Rounding.
// Deductions cannot exceed TaxInclusive + ChargeTotal before rounding, and the
// final Payable cannot be negative. Errors return a zero result. Neither errors
// nor success modify caller data. This is not complete document validation.
func Totals(in TotalsInput) (TotalsResult, error) {
	if len(in.Lines) == 0 {
		return TotalsResult{}, fmt.Errorf("calc: totals: at least one line is required")
	}
	if err := totalsAmounts(
		namedAmount{"AllowanceTotal", in.AllowanceTotal},
		namedAmount{"ChargeTotal", in.ChargeTotal},
		namedAmount{"PrepaidTotal", in.PrepaidTotal},
	); err != nil {
		return TotalsResult{}, fmt.Errorf("calc: totals: %w", err)
	}
	if !in.Rounding.Equal(round2(in.Rounding)) {
		return TotalsResult{}, fmt.Errorf("calc: totals: Rounding must be cent-rounded")
	}
	out := TotalsResult{
		AllowanceTotal: in.AllowanceTotal, ChargeTotal: in.ChargeTotal,
		PrepaidTotal: in.PrepaidTotal, Rounding: in.Rounding,
	}
	for i, line := range in.Lines {
		scheme, err := catalog.SchemeFor(line.Affectation)
		if err != nil {
			return TotalsResult{}, fmt.Errorf("calc: totals: line %d: %w", i+1, err)
		}
		discount, err := totalsLineAmounts(line)
		if err != nil {
			return TotalsResult{}, fmt.Errorf("calc: totals: line %d: %w", i+1, err)
		}
		switch scheme.ID {
		case catalog.SchemeFree.ID:
			out.FreeAmount = out.FreeAmount.Add(line.TaxBase)
			out.FreeIGV = out.FreeIGV.Add(line.IGV)
			continue
		case catalog.SchemeIGV.ID:
			out.TaxedAmount = out.TaxedAmount.Add(line.TaxBase)
			out.IGV = out.IGV.Add(line.IGV)
		case catalog.SchemeIVAP.ID:
			out.IVAPBase = out.IVAPBase.Add(line.TaxBase)
			out.IVAP = out.IVAP.Add(line.IGV)
		case catalog.SchemeExonerated.ID:
			out.ExoneratedAmount = out.ExoneratedAmount.Add(line.TaxBase)
		case catalog.SchemeUnaffected.ID:
			out.UnaffectedAmount = out.UnaffectedAmount.Add(line.TaxBase)
		case catalog.SchemeExport.ID:
			out.ExportAmount = out.ExportAmount.Add(line.TaxBase)
		}
		out.ISC = out.ISC.Add(line.ISC)
		out.ISCBase = out.ISCBase.Add(line.ISCBase)
		out.OtherTaxes = out.OtherTaxes.Add(line.OtherTax)
		out.OtherTaxesBase = out.OtherTaxesBase.Add(line.OtherTaxBase)
		out.ICBPER = out.ICBPER.Add(line.ICBPER)
		out.TotalTaxes = out.TotalTaxes.Add(line.TotalTaxes)
		out.LineExtension = out.LineExtension.Add(line.LineAmount)
		out.AllowanceTotal = out.AllowanceTotal.Add(discount)
	}
	out.TaxInclusive = out.LineExtension.Add(out.TotalTaxes)
	out.Payable = out.TaxInclusive.Add(out.ChargeTotal).Sub(out.AllowanceTotal).Sub(out.PrepaidTotal)
	if out.Payable.IsNegative() {
		return TotalsResult{}, fmt.Errorf("calc: totals: deductions exceed tax-inclusive amount plus charges")
	}
	out.Payable = out.Payable.Add(out.Rounding)
	if out.Payable.IsNegative() {
		return TotalsResult{}, fmt.Errorf("calc: totals: Payable cannot be negative")
	}
	return out, nil
}

type namedAmount struct {
	name   string
	amount decimal.Decimal
}

func totalsAmounts(amounts ...namedAmount) error {
	for _, a := range amounts {
		if a.amount.IsNegative() || !a.amount.Equal(round2(a.amount)) {
			return fmt.Errorf("%s must be cent-rounded and nonnegative", a.name)
		}
	}
	return nil
}

// totalsLineAmounts validates monetary amounts, not rates, quantity or tax
// eligibility. Unit prices and per-bag factors need not be cent-rounded.
// Manually prepared taxes are accepted without rate inference.
func totalsLineAmounts(l model.InvoiceLine) (decimal.Decimal, error) {
	for _, a := range []namedAmount{
		{"UnitValue", l.UnitValue}, {"UnitPrice", l.UnitPrice}, {"ICBPERFactor", l.ICBPERFactor},
	} {
		if a.amount.IsNegative() {
			return decimal.Zero, fmt.Errorf("%s cannot be negative", a.name)
		}
	}
	if err := totalsAmounts(
		namedAmount{"LineAmount", l.LineAmount}, namedAmount{"TaxBase", l.TaxBase},
		namedAmount{"IGV", l.IGV}, namedAmount{"TotalTaxes", l.TotalTaxes},
		namedAmount{"ISC", l.ISC}, namedAmount{"ISCBase", l.ISCBase},
		namedAmount{"OtherTax", l.OtherTax}, namedAmount{"OtherTaxBase", l.OtherTaxBase},
		namedAmount{"ICBPER", l.ICBPER},
	); err != nil {
		return decimal.Zero, err
	}
	if !l.TotalTaxes.Equal(l.IGV.Add(l.ISC).Add(l.OtherTax).Add(l.ICBPER)) {
		return decimal.Zero, fmt.Errorf("TotalTaxes must equal IGV + ISC + OtherTax + ICBPER")
	}
	if l.Affectation.Free() && (len(l.AllowanceCharges) != 0 ||
		!l.ISC.IsZero() || !l.ISCBase.IsZero() || !l.OtherTax.IsZero() || !l.OtherTaxBase.IsZero() || !l.ICBPER.IsZero()) {
		return decimal.Zero, fmt.Errorf("free operations cannot carry allowances, ISC, other taxes or ICBPER")
	}
	// Use Line's affectation semantics, without deriving tax from a rate.
	if igvPercent(l.Affectation, decimal.Zero).IsZero() && !l.IGV.IsZero() {
		return decimal.Zero, fmt.Errorf("affectation does not support IGV")
	}
	discount := decimal.Zero
	for _, ac := range l.AllowanceCharges {
		if ac.Charge || (ac.Reason != catalog.DiscountItemTaxBase && ac.Reason != catalog.DiscountItem) {
			return decimal.Zero, fmt.Errorf("only discount reasons 00 and 01 are supported")
		}
		if err := totalsAmounts(namedAmount{"discount Amount", ac.Amount}, namedAmount{"discount BaseAmount", ac.BaseAmount}); err != nil {
			return decimal.Zero, err
		}
		if ac.Amount.GreaterThan(ac.BaseAmount) {
			return decimal.Zero, fmt.Errorf("discount exceeds its BaseAmount")
		}
		if ac.Reason == catalog.DiscountItem {
			discount = discount.Add(ac.Amount)
		}
	}
	if discount.GreaterThan(l.LineAmount) {
		return decimal.Zero, fmt.Errorf("non-base discounts exceed LineAmount")
	}
	return discount, nil
}

// Apply returns an invoice copy with only its monetary totals replaced. It does
// not populate lines, allowance/prepayment records, perception, detraction,
// payment terms or legends. Existing slices/pointers remain shared, unchanged.
// Totals is opt-in: model, render, build and send APIs never call it implicitly.
func (r TotalsResult) Apply(inv model.Invoice) model.Invoice {
	inv.TaxedAmount = r.TaxedAmount
	inv.ExoneratedAmount = r.ExoneratedAmount
	inv.UnaffectedAmount = r.UnaffectedAmount
	inv.ExportAmount = r.ExportAmount
	inv.FreeAmount = r.FreeAmount
	inv.FreeIGV = r.FreeIGV
	inv.IGV = r.IGV
	inv.ISC = r.ISC
	inv.ISCBase = r.ISCBase
	inv.IVAP = r.IVAP
	inv.IVAPBase = r.IVAPBase
	inv.OtherTaxes = r.OtherTaxes
	inv.OtherTaxesBase = r.OtherTaxesBase
	inv.ICBPER = r.ICBPER
	inv.TotalTaxes = r.TotalTaxes
	inv.LineExtension = r.LineExtension
	inv.TaxInclusive = r.TaxInclusive
	inv.AllowanceTotal = r.AllowanceTotal
	inv.ChargeTotal = r.ChargeTotal
	inv.PrepaidTotal = r.PrepaidTotal
	inv.Rounding = r.Rounding
	inv.Payable = r.Payable
	return inv
}
