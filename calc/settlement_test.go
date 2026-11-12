package calc_test

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/calc"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

func sdec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func eq(t *testing.T, name string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(sdec(want)) {
		t.Errorf("%s = %s, want %s", name, got, want)
	}
}

// Same input as the reference PHP LiquidacionCompraCalculator sample: a taxed
// and an exonerated line at the general 1.5 % income tax retention.
func TestSettlement(t *testing.T) {
	r, err := calc.Settlement(calc.SettlementInput{
		IncomeTaxPercent: sdec("1.5"),
		Lines: []calc.SettlementLineInput{
			{Description: "CAFE", Quantity: sdec("100"), UnitValue: sdec("10")},
			{Description: "CACAO", Quantity: sdec("20"), UnitValue: sdec("10"), Affectation: catalog.ExoneratedOnerous},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "TaxedAmount", r.TaxedAmount, "1000")
	eq(t, "ExoneratedAmount", r.ExoneratedAmount, "200")
	eq(t, "UnaffectedAmount", r.UnaffectedAmount, "0")
	eq(t, "IGV", r.IGV, "180")
	eq(t, "IncomeTaxBase", r.IncomeTaxBase, "1200")
	eq(t, "IncomeTax", r.IncomeTax, "18")
	eq(t, "LineExtension", r.LineExtension, "1200")
	eq(t, "TaxInclusive", r.TaxInclusive, "1380")
	eq(t, "Payable", r.Payable, "1182")

	taxed, exo := r.Lines[0], r.Lines[1]
	if taxed.Affectation != catalog.TaxedOnerous {
		t.Errorf("default affectation = %q", taxed.Affectation)
	}
	eq(t, "taxed IGVPercent", taxed.IGVPercent, "18")
	eq(t, "taxed IGV", taxed.IGV, "180")
	eq(t, "taxed UnitPrice", taxed.UnitPrice, "11.8")
	eq(t, "taxed IncomeTax", taxed.IncomeTax, "15")
	eq(t, "exo IGVPercent", exo.IGVPercent, "0")
	eq(t, "exo IGV", exo.IGV, "0")
	eq(t, "exo UnitPrice", exo.UnitPrice, "10")
	eq(t, "exo IncomeTax", exo.IncomeTax, "3")
}

// Rounding: line amounts are cent-rounded before taxes; the document IGV is
// computed from the summed taxed base (rule 4290), not by adding line IGVs.
func TestSettlementRounding(t *testing.T) {
	r, err := calc.Settlement(calc.SettlementInput{
		IncomeTaxPercent: sdec("4"),
		Lines: []calc.SettlementLineInput{
			{Quantity: sdec("3"), UnitValue: sdec("0.335")},  // 1.005 -> 1.01, IGV 0.18
			{Quantity: sdec("7"), UnitValue: sdec("1.4321")}, // 10.0247 -> 10.02, IGV 1.80
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "line 1 amount", r.Lines[0].LineAmount, "1.01")
	eq(t, "line 1 IGV", r.Lines[0].IGV, "0.18")
	eq(t, "line 2 amount", r.Lines[1].LineAmount, "10.02")
	eq(t, "line 2 IGV", r.Lines[1].IGV, "1.8")
	eq(t, "line 2 UnitPrice", r.Lines[1].UnitPrice, "1.6885714286")
	eq(t, "TaxedAmount", r.TaxedAmount, "11.03")
	eq(t, "IGV", r.IGV, "1.99")             // round2(11.03 * 0.18) = 1.9854, not 0.18 + 1.80
	eq(t, "IncomeTax", r.IncomeTax, "0.44") // 0.04 + 0.40
	eq(t, "Payable", r.Payable, "10.59")
}

func TestSettlementApply(t *testing.T) {
	r, err := calc.Settlement(calc.SettlementInput{Lines: []calc.SettlementLineInput{{Quantity: sdec("1"), UnitValue: sdec("100")}}})
	if err != nil {
		t.Fatal(err)
	}
	p := r.Apply(model.PurchaseSettlement{Series: "L001", Number: 1})
	if p.Series != "L001" || len(p.Lines) != 1 || !p.Payable.Equal(sdec("100")) || !p.IGV.Equal(sdec("18")) {
		t.Errorf("Apply = %+v", p)
	}
	r.Lines[0].Description = "mutated"
	if p.Lines[0].Description == "mutated" {
		t.Error("Apply must copy the lines")
	}
}

func TestSettlementErrors(t *testing.T) {
	line := calc.SettlementLineInput{Quantity: sdec("1"), UnitValue: sdec("1")}
	for name, in := range map[string]calc.SettlementInput{
		"no lines":      {},
		"export":        {Lines: []calc.SettlementLineInput{{Quantity: sdec("1"), UnitValue: sdec("1"), Affectation: catalog.Export}}},
		"zero quantity": {Lines: []calc.SettlementLineInput{{UnitValue: sdec("1")}}},
		"zero value":    {Lines: []calc.SettlementLineInput{{Quantity: sdec("1")}}},
		"negative rate": {Lines: []calc.SettlementLineInput{line}, IncomeTaxPercent: sdec("-1")},
		"rate 100":      {Lines: []calc.SettlementLineInput{line}, IncomeTaxPercent: sdec("100")},
		"negative IGV":  {Lines: []calc.SettlementLineInput{line}, IGVPercent: sdec("-18")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := calc.Settlement(in); err == nil || !strings.HasPrefix(err.Error(), "calc: settlement") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
