// Package calc computes SUNAT document lines through Line and offers opt-in
// document aggregation through Totals. Neither helper is called implicitly by
// model, rendering, building or sending APIs; manual amounts remain supported.
//
// All money is shopspring/decimal. Rounding rules, which follow SUNAT's
// validation tolerances and Greenter's behaviour:
//
//   - Unit values and unit prices keep up to 10 decimals (rounded half away
//     from zero, which is decimal.Round's behaviour).
//   - Line amounts (valor de venta, base, IGV, ISC, ICBPER, discounts) are
//     rounded to 2 decimals. The line value is round2(quantity * unit value),
//     using the already rounded 10-decimal unit value.
//   - Line IGV is round2(line base * rate): it is computed from the rounded
//     base, never from the unrounded product.
//   - Exception, retail prices: when the caller gives UnitPrice (tax
//     included) on a plain taxed line (no discount, ISC or other tax), the
//     customer must be charged exactly round2(quantity * unit price). The line
//     IGV is then that total minus the line value, which can differ by one
//     cent from round2(base * rate) and stays within SUNAT's tolerance (3103).
//     Without it a S/ 6.00 price would be billed as 5.08 + 0.91 = 5.99.
//
// Free operations (transferencias gratuitas, Affectation.Free) carry a
// reference value and, for taxed ones, a notional IGV in the line. Totals keeps
// them separate in FreeAmount/FreeIGV, outside collectible taxes and Payable.
// Totals sums already-cent-rounded amounts with explicit non-base adjustments;
// it does not allocate base-changing global adjustments or infer tax policy.
package calc

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

const divPrecision = 20

var (
	hundred     = decimal.NewFromInt(100)
	one         = decimal.NewFromInt(1)
	defaultIGV  = decimal.NewFromInt(18)
	defaultIVAP = decimal.NewFromInt(4)
)

func round2(d decimal.Decimal) decimal.Decimal  { return d.Round(2) }
func round5(d decimal.Decimal) decimal.Decimal  { return d.Round(5) }
func round10(d decimal.Decimal) decimal.Decimal { return d.Round(10) }

// LineInput is what the caller knows about one detail line.
//
// Price: set exactly one of UnitPrice (the retail price, with every tax
// included: IGV and ISC) or UnitValue (the price without taxes). Line
// derives the other one.
//
// Free operations: the amount given is the reference price of the goods. Line
// reports it, without IGV, in model.InvoiceLine.UnitPrice with price type 02
// and sets UnitValue to zero, as SUNAT requires.
type LineInput struct {
	ProductCode string
	Unit        string // UN/ECE rec 20; defaults to "NIU"
	Description string
	Quantity    decimal.Decimal // must be positive

	UnitPrice decimal.Decimal // with taxes; exclusive with UnitValue
	UnitValue decimal.Decimal // without taxes; exclusive with UnitPrice

	// Affectation is SUNAT catalog 07. It defaults to catalog.TaxedOnerous.
	Affectation catalog.IGVAffectation
	// IGVPercent is the IGV rate, e.g. 18 or 10.5. It defaults to 18 for taxed
	// operations (including taxed free ones) and to 4 for IVAP, and is ignored
	// (zero) for exonerated, unaffected, export and non-taxed free operations.
	IGVPercent decimal.Decimal

	// Line discount, optional. Set at most one of DiscountFactor (a fraction of
	// the line value, 0.10 for 10 %) or DiscountAmount (an amount without IGV).
	// DiscountReason is catalog.DiscountItemTaxBase (00, the default) when the
	// discount reduces the taxable base, or catalog.DiscountItem (01) when it
	// does not; in that case the line value and taxes are untouched and the
	// caller must deduct the discount from the document total as AllowanceTotal.
	DiscountFactor decimal.Decimal
	DiscountAmount decimal.Decimal
	DiscountReason catalog.AllowanceChargeReason

	// ISC, optional. ISCPercent is the rate of system 01 "al valor"
	// (catalog.ISCFixedAmount, despite its name); ISCFixedAmount is the amount
	// per unit of system 02 "monto fijo" (catalog.ISCMinimum). Set at most
	// one; ISCSystem is inferred when empty. System 03 is not supported.
	// ISC is computed over the line value and enters the IGV base.
	ISCPercent     decimal.Decimal
	ISCFixedAmount decimal.Decimal
	ISCSystem      catalog.ISCSystem

	// OtherTaxPercent is the rate of another tax (scheme 9999) over the line
	// value, optional.
	OtherTaxPercent decimal.Decimal

	// ICBPERFactor is the plastic bag tax amount per unit; ICBPER is
	// quantity * factor.
	ICBPERFactor decimal.Decimal

	GS1Code          string
	SunatProductCode string
	Attributes       []model.ItemAttribute
}

// Line computes a model.InvoiceLine from in: UnitValue, UnitPrice, PriceType,
// TaxBase, IGV, TotalTaxes, LineAmount, the line's allowance entries and its
// ISC, other tax and ICBPER amounts.
//
// It rejects non-positive quantities, ambiguous or missing prices, invalid
// affectations and discounts above the line value. Free operations cannot
// carry discounts, ISC, other taxes or ICBPER.
func Line(in LineInput) (model.InvoiceLine, error) {
	aff := in.Affectation
	if aff == "" {
		aff = catalog.TaxedOnerous
	}
	if _, err := catalog.SchemeFor(aff); err != nil {
		return model.InvoiceLine{}, fmt.Errorf("calc: %w", err)
	}
	switch {
	case !in.Quantity.IsPositive():
		return model.InvoiceLine{}, fmt.Errorf("calc: quantity must be positive")
	case in.UnitPrice.IsNegative() || in.UnitValue.IsNegative():
		return model.InvoiceLine{}, fmt.Errorf("calc: unit price and unit value cannot be negative")
	case in.UnitPrice.IsZero() && in.UnitValue.IsZero():
		return model.InvoiceLine{}, fmt.Errorf("calc: unit price or unit value is required")
	case !in.UnitPrice.IsZero() && !in.UnitValue.IsZero():
		return model.InvoiceLine{}, fmt.Errorf("calc: set either unit price or unit value, not both")
	case in.IGVPercent.IsNegative():
		return model.InvoiceLine{}, fmt.Errorf("calc: IGV percent cannot be negative")
	}
	free := aff.Free()
	hasDiscount := !in.DiscountFactor.IsZero() || !in.DiscountAmount.IsZero()
	isc, err := newISC(in)
	if err != nil {
		return model.InvoiceLine{}, err
	}
	if free && (hasDiscount || isc.on() || !in.OtherTaxPercent.IsZero() || !in.ICBPERFactor.IsZero()) {
		return model.InvoiceLine{}, fmt.Errorf("calc: free operations cannot carry discounts, ISC, other taxes or ICBPER")
	}
	if in.OtherTaxPercent.IsNegative() || in.ICBPERFactor.IsNegative() {
		return model.InvoiceLine{}, fmt.Errorf("calc: other tax percent and ICBPER factor cannot be negative")
	}

	percent := igvPercent(aff, in.IGVPercent)
	rate := percent.Div(hundred)

	// Unit value (without taxes) and unit price (with taxes).
	var unitValue, unitPrice decimal.Decimal
	if !in.UnitValue.IsZero() {
		unitValue = round10(in.UnitValue)
		unitPrice = round10(unitValue.Add(isc.perUnit(unitValue)).Mul(one.Add(rate)))
	} else {
		unitPrice = round10(in.UnitPrice)
		pre, err := isc.removeFrom(unitPrice.DivRound(one.Add(rate), divPrecision))
		if err != nil {
			return model.InvoiceLine{}, err
		}
		unitValue = round10(pre)
	}

	out := model.InvoiceLine{
		ProductCode:      in.ProductCode,
		Unit:             in.Unit,
		Description:      in.Description,
		Quantity:         in.Quantity,
		UnitValue:        unitValue,
		UnitPrice:        unitPrice,
		PriceType:        catalog.UnitPriceWithTax,
		Affectation:      aff,
		IGVPercent:       percent,
		GS1Code:          in.GS1Code,
		SunatProductCode: in.SunatProductCode,
		Attributes:       in.Attributes,
	}
	if out.Unit == "" {
		out.Unit = "NIU"
	}

	gross := round2(in.Quantity.Mul(unitValue))

	if free {
		out.UnitValue = decimal.Zero
		out.UnitPrice = unitValue
		out.PriceType = catalog.ReferenceValue
		out.LineAmount = gross
		out.TaxBase = gross
		out.IGV = round2(gross.Mul(rate))
		out.TotalTaxes = out.IGV
		return out, nil
	}

	out.LineAmount = gross
	if hasDiscount {
		ac, err := lineDiscount(in, gross)
		if err != nil {
			return model.InvoiceLine{}, err
		}
		out.AllowanceCharges = []model.AllowanceCharge{ac}
		if ac.Reason == catalog.DiscountItemTaxBase {
			out.LineAmount = gross.Sub(ac.Amount)
		}
	}

	if isc.on() {
		out.ISCBase = out.LineAmount
		out.ISC = isc.amount(out.LineAmount, in.Quantity)
		out.ISCPercent = isc.percent
		out.ISCSystem = isc.system
	}
	out.TaxBase = out.LineAmount.Add(out.ISC)
	out.IGV = round2(out.TaxBase.Mul(rate))
	// A tax-included price is what the customer pays: keep the line total at
	// round2(quantity × unit price) and take the IGV as the difference.
	if in.UnitValue.IsZero() && rate.IsPositive() && !hasDiscount && !isc.on() && in.OtherTaxPercent.IsZero() {
		if igv := round2(in.Quantity.Mul(unitPrice)).Sub(out.TaxBase); !igv.IsNegative() {
			out.IGV = igv
		}
	}
	if !in.OtherTaxPercent.IsZero() {
		out.OtherTaxBase = out.LineAmount
		out.OtherTaxPercent = in.OtherTaxPercent
		out.OtherTax = round2(out.LineAmount.Mul(in.OtherTaxPercent).Div(hundred))
	}
	if !in.ICBPERFactor.IsZero() {
		out.ICBPERFactor = in.ICBPERFactor
		out.ICBPER = round2(in.Quantity.Mul(in.ICBPERFactor))
	}
	out.TotalTaxes = out.IGV.Add(out.ISC).Add(out.OtherTax).Add(out.ICBPER)
	return out, nil
}

// igvPercent returns the rate in percent a line with affectation aff uses.
func igvPercent(aff catalog.IGVAffectation, given decimal.Decimal) decimal.Decimal {
	var def decimal.Decimal
	switch {
	case aff == catalog.TaxedIVAP:
		def = defaultIVAP
	case aff == catalog.TaxedOnerous, aff.Free() && strings.HasPrefix(string(aff), "1"):
		// Codes 11-16 are the taxed free operations.
		def = defaultIGV
	default:
		return decimal.Zero
	}
	if given.IsZero() {
		return def
	}
	return given
}

// lineDiscount builds the allowance entry of a line discount.
func lineDiscount(in LineInput, gross decimal.Decimal) (model.AllowanceCharge, error) {
	if !in.DiscountFactor.IsZero() && !in.DiscountAmount.IsZero() {
		return model.AllowanceCharge{}, fmt.Errorf("calc: set either discount factor or discount amount, not both")
	}
	reason := in.DiscountReason
	if reason == "" {
		reason = catalog.DiscountItemTaxBase
	}
	if reason != catalog.DiscountItemTaxBase && reason != catalog.DiscountItem {
		return model.AllowanceCharge{}, fmt.Errorf("calc: line discount reason must be %q or %q, got %q",
			catalog.DiscountItemTaxBase, catalog.DiscountItem, reason)
	}
	if in.DiscountFactor.IsNegative() || in.DiscountAmount.IsNegative() {
		return model.AllowanceCharge{}, fmt.Errorf("calc: discount cannot be negative")
	}
	if !gross.IsPositive() {
		return model.AllowanceCharge{}, fmt.Errorf("calc: cannot discount a line with no value")
	}
	ac := model.AllowanceCharge{Reason: reason, BaseAmount: gross}
	if !in.DiscountFactor.IsZero() {
		if in.DiscountFactor.GreaterThan(one) {
			return model.AllowanceCharge{}, fmt.Errorf("calc: discount factor %s is above 1 (use 0.10 for 10 %%)", in.DiscountFactor)
		}
		ac.Factor = in.DiscountFactor
		ac.Amount = round2(gross.Mul(in.DiscountFactor))
	} else {
		ac.Amount = round2(in.DiscountAmount)
		ac.Factor = round5(ac.Amount.DivRound(gross, divPrecision))
	}
	if ac.Amount.GreaterThan(gross) {
		return model.AllowanceCharge{}, fmt.Errorf("calc: discount %s exceeds the line value %s", ac.Amount, gross)
	}
	return ac, nil
}

// iscSpec is the validated ISC configuration of a line.
type iscSpec struct {
	system  catalog.ISCSystem
	percent decimal.Decimal // system 01
	fixed   decimal.Decimal // system 02, per unit
}

func (s iscSpec) on() bool { return s.system != "" }

func newISC(in LineInput) (iscSpec, error) {
	pct, fixed := in.ISCPercent, in.ISCFixedAmount
	if pct.IsNegative() || fixed.IsNegative() {
		return iscSpec{}, fmt.Errorf("calc: ISC percent and fixed amount cannot be negative")
	}
	if in.ISCSystem == catalog.ISCRetail {
		return iscSpec{}, fmt.Errorf("calc: ISC system %q (precio de venta al público) is not supported; fill the ISC fields manually", catalog.ISCRetail)
	}
	switch {
	case pct.IsZero() && fixed.IsZero():
		return iscSpec{}, nil
	case !pct.IsZero() && !fixed.IsZero():
		return iscSpec{}, fmt.Errorf("calc: set either ISC percent or ISC fixed amount, not both")
	case !pct.IsZero():
		if in.ISCSystem != "" && in.ISCSystem != catalog.ISCFixedAmount {
			return iscSpec{}, fmt.Errorf("calc: ISC percent requires system %q, got %q", catalog.ISCFixedAmount, in.ISCSystem)
		}
		return iscSpec{system: catalog.ISCFixedAmount, percent: pct}, nil
	default:
		if in.ISCSystem != "" && in.ISCSystem != catalog.ISCMinimum {
			return iscSpec{}, fmt.Errorf("calc: ISC fixed amount requires system %q, got %q", catalog.ISCMinimum, in.ISCSystem)
		}
		return iscSpec{system: catalog.ISCMinimum, fixed: fixed}, nil
	}
}

// perUnit is the ISC added to a unit value.
func (s iscSpec) perUnit(unitValue decimal.Decimal) decimal.Decimal {
	switch s.system {
	case catalog.ISCFixedAmount:
		return unitValue.Mul(s.percent).Div(hundred)
	case catalog.ISCMinimum:
		return s.fixed
	}
	return decimal.Zero
}

// removeFrom takes the ISC out of a price that already includes it but not IGV.
func (s iscSpec) removeFrom(price decimal.Decimal) (decimal.Decimal, error) {
	switch s.system {
	case catalog.ISCFixedAmount:
		return price.DivRound(one.Add(s.percent.Div(hundred)), divPrecision), nil
	case catalog.ISCMinimum:
		v := price.Sub(s.fixed)
		if v.IsNegative() {
			return decimal.Zero, fmt.Errorf("calc: unit price is below the ISC fixed amount")
		}
		return v, nil
	}
	return price, nil
}

// amount is the ISC of a line with the given value and quantity.
func (s iscSpec) amount(lineAmount, quantity decimal.Decimal) decimal.Decimal {
	switch s.system {
	case catalog.ISCFixedAmount:
		return round2(lineAmount.Mul(s.percent).Div(hundred))
	case catalog.ISCMinimum:
		return round2(quantity.Mul(s.fixed))
	}
	return decimal.Zero
}
