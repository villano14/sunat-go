package calc_test

import (
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/villano14/sunat-go/calc"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// Compare every public field, using numeric equality rather than decimal scale.
func assertFields(t *testing.T, got, want any) {
	t.Helper()
	g, w := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := 0; i < g.NumField(); i++ {
		name := g.Type().Field(i).Name
		gv, wv := g.Field(i).Interface(), w.Field(i).Interface()
		if gd, ok := gv.(decimal.Decimal); ok {
			if !gd.Equal(wv.(decimal.Decimal)) {
				t.Errorf("%s = %s, want %s", name, gd, wv)
			}
		} else if ga, ok := gv.([]model.AllowanceCharge); ok {
			wa := wv.([]model.AllowanceCharge)
			if len(ga) != len(wa) {
				t.Errorf("%s = %+v, want %+v", name, ga, wa)
				continue
			}
			for j := range ga {
				assertFields(t, ga[j], wa[j])
			}
		} else if !reflect.DeepEqual(gv, wv) {
			t.Errorf("%s = %+v, want %+v", name, gv, wv)
		}
	}
}

func checkedLine(t *testing.T, in calc.LineInput) model.InvoiceLine {
	t.Helper()
	before := in
	before.Attributes = append([]model.ItemAttribute(nil), in.Attributes...)
	got, err := calc.Line(in)
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if !reflect.DeepEqual(in, before) {
		t.Fatal("Line mutated its input")
	}
	return got
}

func TestLinePricesAndRounding(t *testing.T) {
	for _, tt := range []struct {
		name, quantity, value, price      string
		wantValue, wantPrice, amount, igv string
	}{
		{"value derives price", "2", "100", "0", "100", "118", "200", "36"},
		{"price derives value", "2", "0", "118", "100", "118", "200", "36"},
		{"unit value half rounds to ten places", "1", "0.12345678905", "0", "0.1234567891", "0.1456790111", "0.12", "0.02"},
		{"unit price rounds up to ten places", "1", "0", "1.180000000059", "1.0000000001", "1.1800000001", "1", "0.18"},
		{"unit price half rounds before deriving value", "1", "0", "1.18000000005", "1.0000000001", "1.1800000001", "1", "0.18"},
		{"unit price rounds down to ten places", "1", "0", "1.179999999951", "1", "1.18", "1", "0.18"},
		{"unit rounding precedes quantity product", "1", "0.00499999996", "0", "0.005", "0.0059", "0.01", "0"},
		{"IGV uses rounded base not raw product", "1", "0.027", "0", "0.027", "0.03186", "0.03", "0.01"},
		{"fractional quantity", "2.5", "0.33333333335", "0", "0.3333333334", "0.3933333334", "0.83", "0.15"},
		{"positive value can round to zero", "1", "0.00000000001", "0", "0", "0", "0", "0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d(tt.quantity), UnitValue: d(tt.value), UnitPrice: d(tt.price)}
			want := model.InvoiceLine{
				Quantity: d(tt.quantity), Unit: "NIU", Affectation: catalog.TaxedOnerous,
				UnitValue: d(tt.wantValue), UnitPrice: d(tt.wantPrice), PriceType: catalog.UnitPriceWithTax,
				LineAmount: d(tt.amount), TaxBase: d(tt.amount), IGVPercent: d("18"), IGV: d(tt.igv), TotalTaxes: d(tt.igv),
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

// A tax-included price must bill exactly quantity × price: the IGV is the
// difference between that total and the line value.
func TestLineRetailPriceKeepsTotal(t *testing.T) {
	for _, tt := range []struct {
		name, quantity, price         string
		wantValue, amount, igv, total string
	}{
		{"6.00 is not billed as 5.99", "1", "6", "5.0847457627", "5.08", "0.92", "6"},
		{"10.00 is not billed as 9.99", "1", "10", "8.4745762712", "8.47", "1.53", "10"},
		{"several units", "3", "6", "5.0847457627", "15.25", "2.75", "18"},
		{"exact price is unchanged", "2", "118", "100", "200", "36", "236"},
		{"fractional quantity", "0.5", "3.5", "2.9661016949", "1.48", "0.27", "1.75"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkedLine(t, calc.LineInput{Quantity: d(tt.quantity), UnitPrice: d(tt.price)})
			want := model.InvoiceLine{
				Quantity: d(tt.quantity), Unit: "NIU", Affectation: catalog.TaxedOnerous,
				UnitValue: d(tt.wantValue), UnitPrice: d(tt.price), PriceType: catalog.UnitPriceWithTax,
				LineAmount: d(tt.amount), TaxBase: d(tt.amount), IGVPercent: d("18"), IGV: d(tt.igv), TotalTaxes: d(tt.igv),
			}
			assertFields(t, got, want)
			if sum := got.LineAmount.Add(got.IGV); !sum.Equal(d(tt.total)) {
				t.Errorf("line total = %s, want %s", sum, tt.total)
			}
		})
	}
}

func TestLineIGVHalfRounding(t *testing.T) {
	for _, tt := range []struct {
		name, value, price, igv string
	}{
		{"below half cent", "0.04", "0.044", "0"},
		{"half cent", "0.05", "0.055", "0.01"},
		{"one and a half cents", "0.15", "0.165", "0.02"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d("1"), UnitValue: d(tt.value), IGVPercent: d("10")}
			want := model.InvoiceLine{
				Quantity: d("1"), Unit: "NIU", Affectation: catalog.TaxedOnerous, PriceType: catalog.UnitPriceWithTax,
				UnitValue: d(tt.value), UnitPrice: d(tt.price), LineAmount: d(tt.value), TaxBase: d(tt.value),
				IGVPercent: d("10"), IGV: d(tt.igv), TotalTaxes: d(tt.igv),
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

func TestLineAffectations(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		aff                        catalog.IGVAffectation
		given, percent, price, igv string
		free, fromPrice            bool
	}{
		{"taxed default", catalog.TaxedOnerous, "0", "18", "118", "36", false, false},
		{"taxed custom rate", catalog.TaxedOnerous, "10.5", "10.5", "110.5", "21", false, false},
		{"IVAP default", catalog.TaxedIVAP, "0", "4", "104", "8", false, false},
		{"IVAP custom", catalog.TaxedIVAP, "5", "5", "105", "10", false, true},
		{"exonerated ignores rate", catalog.ExoneratedOnerous, "18", "0", "100", "0", false, false},
		{"unaffected ignores rate", catalog.UnaffectedOnerous, "18", "0", "100", "0", false, false},
		{"export ignores rate", catalog.Export, "18", "0", "100", "0", false, true},
		{"free taxed reference from price", catalog.TaxedFree, "0", "18", "118", "36", true, true},
		{"donation custom rate", catalog.TaxedDonation, "10.5", "10.5", "110.5", "21", true, false},
		{"withdrawal", catalog.TaxedWithdrawal, "0", "18", "118", "36", true, false},
		{"advertising", catalog.TaxedAdvertising, "0", "18", "118", "36", true, false},
		{"bonus", catalog.TaxedBonus, "0", "18", "118", "36", true, false},
		{"workers", catalog.TaxedWorkers, "0", "18", "118", "36", true, false},
		{"free exonerated", catalog.ExoneratedFree, "18", "0", "100", "0", true, true},
		{"unaffected bonus and free alias", catalog.UnaffectedFree, "18", "0", "100", "0", true, false},
		{"unaffected withdrawal", catalog.UnaffectedWithdr, "18", "0", "100", "0", true, false},
		{"samples", catalog.UnaffectedSamples, "18", "0", "100", "0", true, false},
		{"union", catalog.UnaffectedUnion, "18", "0", "100", "0", true, false},
		{"prize", catalog.UnaffectedPrize, "18", "0", "100", "0", true, false},
		{"unaffected advertising", catalog.UnaffectedAdv, "18", "0", "100", "0", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d("2"), UnitValue: d("100"), Affectation: tt.aff, IGVPercent: d(tt.given)}
			if tt.fromPrice {
				in.UnitValue, in.UnitPrice = decimal.Zero, d(tt.price)
			}
			want := model.InvoiceLine{
				Quantity: d("2"), Unit: "NIU", Affectation: tt.aff, PriceType: catalog.UnitPriceWithTax,
				UnitValue: d("100"), UnitPrice: d(tt.price), LineAmount: d("200"), TaxBase: d("200"),
				IGVPercent: d(tt.percent), IGV: d(tt.igv), TotalTaxes: d(tt.igv),
			}
			if tt.free {
				want.UnitValue, want.UnitPrice, want.PriceType = decimal.Zero, d("100"), catalog.ReferenceValue
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

func TestLineMetadata(t *testing.T) {
	in := calc.LineInput{
		Quantity: d("2"), UnitValue: d("100"), ProductCode: "SKU-1", Unit: "ZZ", Description: "Service",
		GS1Code: "12345678", SunatProductCode: "81111500",
		Attributes: []model.ItemAttribute{{Code: "7000", Name: "Property", Value: "Value", Duration: 2}},
	}
	want := model.InvoiceLine{
		Quantity: d("2"), UnitValue: d("100"), UnitPrice: d("118"), PriceType: catalog.UnitPriceWithTax,
		Affectation: catalog.TaxedOnerous, IGVPercent: d("18"), LineAmount: d("200"), TaxBase: d("200"), IGV: d("36"), TotalTaxes: d("36"),
		ProductCode: "SKU-1", Unit: "ZZ", Description: "Service", GS1Code: "12345678", SunatProductCode: "81111500",
		Attributes: []model.ItemAttribute{{Code: "7000", Name: "Property", Value: "Value", Duration: 2}},
	}
	assertFields(t, checkedLine(t, in), want)
}

func TestLineDiscounts(t *testing.T) {
	for _, tt := range []struct {
		name, factor, amount                    string
		reason                                  catalog.AllowanceChargeReason
		wantFactor, wantAmount, lineAmount, igv string
	}{
		{"default taxable factor", "0.1", "0", "", "0.1", "0.3", "2.7", "0.49"},
		{"amount derives five place factor", "0", "1", catalog.DiscountItemTaxBase, "0.33333", "1", "2", "0.36"},
		{"non-base amount leaves taxes intact", "0", "1", catalog.DiscountItem, "0.33333", "1", "3", "0.54"},
		{"non-base factor leaves taxes intact", "0.1", "0", catalog.DiscountItem, "0.1", "0.3", "3", "0.54"},
		{"amount half rounds away from zero", "0", "0.005", "", "0.00333", "0.01", "2.99", "0.54"},
		{"factor amount rounds to cents", "0.0016666667", "0", "", "0.0016666667", "0.01", "2.99", "0.54"},
		{"full discount factor", "1", "0", "", "1", "3", "0", "0"},
		{"full discount amount", "0", "3", "", "1", "3", "0", "0"},
		{"amount compared after cent rounding", "0", "3.004", "", "1", "3", "0", "0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d("3"), UnitValue: d("1"), DiscountFactor: d(tt.factor), DiscountAmount: d(tt.amount), DiscountReason: tt.reason}
			reason := tt.reason
			if reason == "" {
				reason = catalog.DiscountItemTaxBase
			}
			want := model.InvoiceLine{
				Quantity: d("3"), Unit: "NIU", UnitValue: d("1"), UnitPrice: d("1.18"), PriceType: catalog.UnitPriceWithTax,
				Affectation: catalog.TaxedOnerous, IGVPercent: d("18"), LineAmount: d(tt.lineAmount), TaxBase: d(tt.lineAmount), IGV: d(tt.igv), TotalTaxes: d(tt.igv),
				AllowanceCharges: []model.AllowanceCharge{{Reason: reason, BaseAmount: d("3"), Factor: d(tt.wantFactor), Amount: d(tt.wantAmount)}},
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

func TestLineISC(t *testing.T) {
	for _, tt := range []struct {
		name, quantity, value, price, percent, fixed        string
		system, wantSystem                                  catalog.ISCSystem
		wantValue, wantPrice, amount, isc, base, igv, total string
	}{
		{"percent inferred", "2", "100", "0", "10", "0", "", catalog.ISCFixedAmount, "100", "129.8", "200", "20", "220", "39.6", "59.6"},
		{"percent explicit from retail price", "2", "0", "129.8", "10", "0", catalog.ISCFixedAmount, catalog.ISCFixedAmount, "100", "129.8", "200", "20", "220", "39.6", "59.6"},
		{"fixed inferred", "2", "100", "0", "0", "3", "", catalog.ISCMinimum, "100", "121.54", "200", "6", "206", "37.08", "43.08"},
		{"fixed explicit from retail price", "2", "0", "121.54", "0", "3", catalog.ISCMinimum, catalog.ISCMinimum, "100", "121.54", "200", "6", "206", "37.08", "43.08"},
		{"percent cents rounding", "3", "0.5", "0", "1", "0", "", catalog.ISCFixedAmount, "0.5", "0.5959", "1.5", "0.02", "1.52", "0.27", "0.29"},
		{"fixed cents rounding", "3", "0.5", "0", "0", "0.005", "", catalog.ISCMinimum, "0.5", "0.5959", "1.5", "0.02", "1.52", "0.27", "0.29"},
		{"retail price equals fixed ISC plus IGV", "2", "0", "3.54", "0", "3", "", catalog.ISCMinimum, "0", "3.54", "0", "6", "6", "1.08", "7.08"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d(tt.quantity), UnitValue: d(tt.value), UnitPrice: d(tt.price), ISCPercent: d(tt.percent), ISCFixedAmount: d(tt.fixed), ISCSystem: tt.system}
			want := model.InvoiceLine{
				Quantity: d(tt.quantity), Unit: "NIU", UnitValue: d(tt.wantValue), UnitPrice: d(tt.wantPrice), PriceType: catalog.UnitPriceWithTax,
				Affectation: catalog.TaxedOnerous, IGVPercent: d("18"), LineAmount: d(tt.amount), TaxBase: d(tt.base), IGV: d(tt.igv), TotalTaxes: d(tt.total),
				ISCBase: d(tt.amount), ISC: d(tt.isc), ISCPercent: d(tt.percent), ISCSystem: tt.wantSystem,
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

func TestLineOtherTaxesAndICBPER(t *testing.T) {
	for _, tt := range []struct {
		name, otherPercent, bagFactor, otherBase, other, bag, total string
	}{
		{"other tax", "1", "0", "1.5", "0.02", "0", "0.29"},
		{"ICBPER", "0", "0.005", "0", "0", "0.02", "0.29"},
		{"both", "1", "0.005", "1.5", "0.02", "0.02", "0.31"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d("3"), UnitValue: d("0.5"), OtherTaxPercent: d(tt.otherPercent), ICBPERFactor: d(tt.bagFactor)}
			want := model.InvoiceLine{
				Quantity: d("3"), Unit: "NIU", UnitValue: d("0.5"), UnitPrice: d("0.59"), PriceType: catalog.UnitPriceWithTax,
				Affectation: catalog.TaxedOnerous, IGVPercent: d("18"), LineAmount: d("1.5"), TaxBase: d("1.5"), IGV: d("0.27"), TotalTaxes: d(tt.total),
				OtherTaxBase: d(tt.otherBase), OtherTaxPercent: d(tt.otherPercent), OtherTax: d(tt.other), ICBPERFactor: d(tt.bagFactor), ICBPER: d(tt.bag),
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

func TestLineDiscountWithAllTaxes(t *testing.T) {
	for _, tt := range []struct {
		name                                        string
		reason                                      catalog.AllowanceChargeReason
		fixed                                       bool
		price, amount, isc, base, igv, other, total string
	}{
		{"taxable discount before percent ISC", catalog.DiscountItemTaxBase, false, "129.8", "180", "18", "198", "35.64", "9", "63.64"},
		{"non-base discount leaves all taxes", catalog.DiscountItem, false, "129.8", "200", "20", "220", "39.6", "10", "70.6"},
		{"fixed ISC remains quantity based", catalog.DiscountItemTaxBase, true, "121.54", "180", "6", "186", "33.48", "9", "49.48"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d("2"), UnitValue: d("100"), DiscountFactor: d("0.1"), DiscountReason: tt.reason, ISCPercent: d("10"), OtherTaxPercent: d("5"), ICBPERFactor: d("0.5")}
			system, percent := catalog.ISCFixedAmount, d("10")
			if tt.fixed {
				in.ISCPercent, in.ISCFixedAmount = decimal.Zero, d("3")
				system, percent = catalog.ISCMinimum, decimal.Zero
			}
			want := model.InvoiceLine{
				Quantity: d("2"), Unit: "NIU", UnitValue: d("100"), UnitPrice: d(tt.price), PriceType: catalog.UnitPriceWithTax,
				Affectation: catalog.TaxedOnerous, IGVPercent: d("18"), LineAmount: d(tt.amount), TaxBase: d(tt.base), IGV: d(tt.igv), TotalTaxes: d(tt.total),
				ISCBase: d(tt.amount), ISC: d(tt.isc), ISCSystem: system, ISCPercent: percent,
				OtherTaxBase: d(tt.amount), OtherTaxPercent: d("5"), OtherTax: d(tt.other), ICBPERFactor: d("0.5"), ICBPER: d("1"),
				AllowanceCharges: []model.AllowanceCharge{{Reason: tt.reason, BaseAmount: d("200"), Factor: d("0.1"), Amount: d("20")}},
			}
			assertFields(t, checkedLine(t, in), want)
		})
	}
}

func TestLineValidationDoesNotMutateInput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(*calc.LineInput)
		message string
	}{
		{"zero quantity", func(in *calc.LineInput) { in.Quantity = decimal.Zero }, "calc: quantity must be positive"},
		{"negative quantity", func(in *calc.LineInput) { in.Quantity = d("-1") }, "calc: quantity must be positive"},
		{"negative value", func(in *calc.LineInput) { in.UnitValue = d("-1") }, "calc: unit price and unit value cannot be negative"},
		{"negative price", func(in *calc.LineInput) { in.UnitValue, in.UnitPrice = decimal.Zero, d("-1") }, "calc: unit price and unit value cannot be negative"},
		{"missing price", func(in *calc.LineInput) { in.UnitValue = decimal.Zero }, "calc: unit price or unit value is required"},
		{"ambiguous price", func(in *calc.LineInput) { in.UnitPrice = d("118") }, "calc: set either unit price or unit value, not both"},
		{"invalid affectation", func(in *calc.LineInput) { in.Affectation = "99" }, "calc: catalog: unsupported IGV affectation \"99\""},
		{"negative IGV", func(in *calc.LineInput) { in.IGVPercent = d("-1") }, "calc: IGV percent cannot be negative"},
		{"ambiguous discount", func(in *calc.LineInput) { in.DiscountFactor, in.DiscountAmount = d("0.1"), d("1") }, "calc: set either discount factor or discount amount, not both"},
		{"invalid discount reason", func(in *calc.LineInput) { in.DiscountAmount, in.DiscountReason = d("1"), catalog.DiscountGlobal }, "calc: line discount reason must be \"00\" or \"01\", got \"03\""},
		{"negative discount factor", func(in *calc.LineInput) { in.DiscountFactor = d("-0.1") }, "calc: discount cannot be negative"},
		{"negative discount amount", func(in *calc.LineInput) { in.DiscountAmount = d("-1") }, "calc: discount cannot be negative"},
		{"discount on rounded zero line", func(in *calc.LineInput) { in.UnitValue, in.DiscountAmount = d("0.001"), d("0.01") }, "calc: cannot discount a line with no value"},
		{"factor above one", func(in *calc.LineInput) { in.DiscountFactor = d("1.00001") }, "calc: discount factor 1.00001 is above 1 (use 0.10 for 10 %)"},
		{"discount above line", func(in *calc.LineInput) { in.DiscountAmount = d("200.005") }, "calc: discount 200.01 exceeds the line value 200"},
		{"negative ISC percent", func(in *calc.LineInput) { in.ISCPercent = d("-1") }, "calc: ISC percent and fixed amount cannot be negative"},
		{"negative fixed ISC", func(in *calc.LineInput) { in.ISCFixedAmount = d("-1") }, "calc: ISC percent and fixed amount cannot be negative"},
		{"unsupported retail ISC", func(in *calc.LineInput) { in.ISCSystem = catalog.ISCRetail }, "calc: ISC system \"03\" (precio de venta al público) is not supported; fill the ISC fields manually"},
		{"ambiguous ISC", func(in *calc.LineInput) { in.ISCPercent, in.ISCFixedAmount = d("1"), d("1") }, "calc: set either ISC percent or ISC fixed amount, not both"},
		{"percent wrong system", func(in *calc.LineInput) { in.ISCPercent, in.ISCSystem = d("1"), catalog.ISCMinimum }, "calc: ISC percent requires system \"01\", got \"02\""},
		{"fixed wrong system", func(in *calc.LineInput) { in.ISCFixedAmount, in.ISCSystem = d("1"), catalog.ISCFixedAmount }, "calc: ISC fixed amount requires system \"02\", got \"01\""},
		{"price below fixed ISC", func(in *calc.LineInput) {
			in.UnitValue, in.UnitPrice, in.ISCFixedAmount = decimal.Zero, d("3.53"), d("3")
		}, "calc: unit price is below the ISC fixed amount"},
		{"negative other tax", func(in *calc.LineInput) { in.OtherTaxPercent = d("-1") }, "calc: other tax percent and ICBPER factor cannot be negative"},
		{"negative bag factor", func(in *calc.LineInput) { in.ICBPERFactor = d("-1") }, "calc: other tax percent and ICBPER factor cannot be negative"},
		{"free with discount factor", func(in *calc.LineInput) { in.Affectation, in.DiscountFactor = catalog.TaxedFree, d("0.1") }, "calc: free operations cannot carry discounts, ISC, other taxes or ICBPER"},
		{"free with discount amount", func(in *calc.LineInput) { in.Affectation, in.DiscountAmount = catalog.TaxedFree, d("1") }, "calc: free operations cannot carry discounts, ISC, other taxes or ICBPER"},
		{"free with percent ISC", func(in *calc.LineInput) { in.Affectation, in.ISCPercent = catalog.TaxedFree, d("1") }, "calc: free operations cannot carry discounts, ISC, other taxes or ICBPER"},
		{"free with fixed ISC", func(in *calc.LineInput) { in.Affectation, in.ISCFixedAmount = catalog.TaxedFree, d("1") }, "calc: free operations cannot carry discounts, ISC, other taxes or ICBPER"},
		{"free with other tax", func(in *calc.LineInput) { in.Affectation, in.OtherTaxPercent = catalog.TaxedFree, d("1") }, "calc: free operations cannot carry discounts, ISC, other taxes or ICBPER"},
		{"free with ICBPER", func(in *calc.LineInput) { in.Affectation, in.ICBPERFactor = catalog.TaxedFree, d("0.5") }, "calc: free operations cannot carry discounts, ISC, other taxes or ICBPER"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := calc.LineInput{Quantity: d("2"), UnitValue: d("100"), Attributes: []model.ItemAttribute{{Name: "Keep", Value: "unchanged"}}}
			tt.change(&in)
			before := in
			before.Attributes = append([]model.ItemAttribute(nil), in.Attributes...)
			got, err := calc.Line(in)
			if err == nil || err.Error() != tt.message {
				t.Errorf("error = %v, want %q", err, tt.message)
			}
			if !reflect.DeepEqual(got, model.InvoiceLine{}) {
				t.Errorf("rejected line = %+v, want zero InvoiceLine", got)
			}
			if !reflect.DeepEqual(in, before) {
				t.Error("rejected input was mutated")
			}
		})
	}
}
