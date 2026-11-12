package calc

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// SettlementLineInput is what the caller knows about one line of a
// Liquidación de compra.
type SettlementLineInput struct {
	ProductCode      string
	SunatProductCode string
	Unit             string // defaults to "KGM" when rendered
	Description      string
	Quantity         decimal.Decimal
	UnitValue        decimal.Decimal        // Valor unitario, without IGV
	Affectation      catalog.IGVAffectation // 10, 20 or 30; defaults to 10
	Attributes       []model.ItemAttribute
}

// SettlementInput holds the lines and rates of a Liquidación de compra.
//
// IncomeTaxPercent is the income tax retention rate, a legal decision left to
// the caller: in 2026 it is 1.5 % in general and 4 % for gold (operation
// 0503), and 0 when the operation does not exceed S/ 700 or the goods are
// primary agricultural products excluded from retention (RS 234-2005/SUNAT and
// its extensions). IGVPercent defaults to 18.
type SettlementInput struct {
	Lines            []SettlementLineInput
	IGVPercent       decimal.Decimal
	IncomeTaxPercent decimal.Decimal
}

// SettlementResult holds the calculated lines and the monetary fields of
// model.PurchaseSettlement.
type SettlementResult struct {
	Lines            []model.PurchaseSettlementLine
	TaxedAmount      decimal.Decimal
	ExoneratedAmount decimal.Decimal
	UnaffectedAmount decimal.Decimal
	IGV              decimal.Decimal
	IncomeTaxBase    decimal.Decimal
	IncomeTax        decimal.Decimal
	LineExtension    decimal.Decimal
	TaxInclusive     decimal.Decimal
	Payable          decimal.Decimal
}

// Settlement computes a Liquidación de compra following the SUNAT validation
// rules of LiquidacionCompra 2.0:
//
//	line amount = round2(quantity × unit value)                      (4288)
//	line IGV    = round2(line amount × IGV rate), taxed lines only     (3103)
//	line income = round2(line amount × income tax rate)              (2464)
//	unit price  = round10((line amount + line IGV) / quantity)       (4287)
//	IGV         = round2(Σ taxed amounts × IGV rate)                 (4290)
//	income tax  = Σ line income tax                                  (4346)
//	TaxInclusive = LineExtension + IGV                               (4310)
//	Payable      = LineExtension − income tax                        (4312)
//
// The income tax base is the whole LineExtension.
func Settlement(in SettlementInput) (SettlementResult, error) {
	if len(in.Lines) == 0 {
		return SettlementResult{}, fmt.Errorf("calc: settlement: at least one line is required")
	}
	igvRate := in.IGVPercent
	if igvRate.IsZero() {
		igvRate = defaultIGV
	}
	if igvRate.IsNegative() || in.IncomeTaxPercent.IsNegative() || in.IncomeTaxPercent.GreaterThanOrEqual(hundred) {
		return SettlementResult{}, fmt.Errorf("calc: settlement: rates must be within 0..100")
	}

	var r SettlementResult
	for i, l := range in.Lines {
		aff := l.Affectation
		if aff == "" {
			aff = catalog.TaxedOnerous
		}
		if aff != catalog.TaxedOnerous && aff != catalog.ExoneratedOnerous && aff != catalog.UnaffectedOnerous {
			return SettlementResult{}, fmt.Errorf("calc: settlement line %d: affectation must be 10, 20 or 30, got %q", i+1, aff)
		}
		if !l.Quantity.IsPositive() || !l.UnitValue.IsPositive() {
			return SettlementResult{}, fmt.Errorf("calc: settlement line %d: quantity and unit value must be positive", i+1)
		}
		unitValue := round10(l.UnitValue)
		amount := round2(l.Quantity.Mul(unitValue))
		line := model.PurchaseSettlementLine{
			ProductCode: l.ProductCode, SunatProductCode: l.SunatProductCode, Unit: l.Unit,
			Description: l.Description, Quantity: l.Quantity, UnitValue: unitValue,
			LineAmount: amount, Affectation: aff, Attributes: l.Attributes,
			IncomeTaxPercent: in.IncomeTaxPercent,
			IncomeTax:        round2(amount.Mul(in.IncomeTaxPercent).Div(hundred)),
		}
		switch aff {
		case catalog.TaxedOnerous:
			line.IGVPercent = igvRate
			line.IGV = round2(amount.Mul(igvRate).Div(hundred))
			r.TaxedAmount = r.TaxedAmount.Add(amount)
		case catalog.ExoneratedOnerous:
			r.ExoneratedAmount = r.ExoneratedAmount.Add(amount)
		default:
			r.UnaffectedAmount = r.UnaffectedAmount.Add(amount)
		}
		line.UnitPrice = round10(amount.Add(line.IGV).DivRound(l.Quantity, divPrecision))
		r.IncomeTax = r.IncomeTax.Add(line.IncomeTax)
		r.Lines = append(r.Lines, line)
	}
	r.LineExtension = r.TaxedAmount.Add(r.ExoneratedAmount).Add(r.UnaffectedAmount)
	r.IncomeTaxBase = r.LineExtension
	r.IGV = round2(r.TaxedAmount.Mul(igvRate).Div(hundred))
	r.TaxInclusive = r.LineExtension.Add(r.IGV)
	r.Payable = r.LineExtension.Sub(r.IncomeTax)
	return r, nil
}

// Apply returns a copy of p with its lines and monetary totals replaced.
func (r SettlementResult) Apply(p model.PurchaseSettlement) model.PurchaseSettlement {
	p.Lines = append([]model.PurchaseSettlementLine(nil), r.Lines...)
	p.TaxedAmount, p.ExoneratedAmount, p.UnaffectedAmount = r.TaxedAmount, r.ExoneratedAmount, r.UnaffectedAmount
	p.IGV, p.IncomeTaxBase, p.IncomeTax = r.IGV, r.IncomeTaxBase, r.IncomeTax
	p.LineExtension, p.TaxInclusive, p.Payable = r.LineExtension, r.TaxInclusive, r.Payable
	return p
}
