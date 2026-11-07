package calc_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/villano14/sunat-go/calc"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// Snapshot nested slices too: a shallow struct copy would hide input mutation.
func copyTotalsInput(in calc.TotalsInput) calc.TotalsInput {
	out := in
	if in.Lines != nil {
		out.Lines = append([]model.InvoiceLine{}, in.Lines...)
	}
	for i, l := range in.Lines {
		if l.AllowanceCharges != nil {
			out.Lines[i].AllowanceCharges = append([]model.AllowanceCharge{}, l.AllowanceCharges...)
		}
		if l.Attributes != nil {
			out.Lines[i].Attributes = append([]model.ItemAttribute{}, l.Attributes...)
		}
	}
	return out
}

func checkTotals(t *testing.T, in calc.TotalsInput, want calc.TotalsResult, message string) {
	t.Helper()
	before := copyTotalsInput(in)
	got, err := calc.Totals(in)
	if message == "" {
		if err != nil {
			t.Fatalf("Totals: %v", err)
		}
	} else {
		if err == nil || err.Error() != message {
			t.Errorf("error = %v, want %q", err, message)
		}
		if !reflect.DeepEqual(got, calc.TotalsResult{}) {
			t.Errorf("rejected result = %+v, want zero TotalsResult", got)
		}
	}
	assertFields(t, got, want)
	if !reflect.DeepEqual(in, before) {
		t.Error("Totals mutated caller input")
	}
}

func totalsLine(t *testing.T, aff catalog.IGVAffectation, value string) model.InvoiceLine {
	t.Helper()
	return checkedLine(t, calc.LineInput{Quantity: d("1"), UnitValue: d(value), Affectation: aff,
		Attributes: []model.ItemAttribute{{Name: "Keep", Value: "unchanged"}}})
}

func TestTotalsCalculationRules(t *testing.T) {
	t.Run("mixed categories and rates remain segregated", func(t *testing.T) {
		lines := []model.InvoiceLine{
			totalsLine(t, catalog.TaxedOnerous, "100"),
			checkedLine(t, calc.LineInput{Quantity: d("1"), UnitValue: d("100"), IGVPercent: d("10.5")}),
			totalsLine(t, catalog.ExoneratedOnerous, "20"),
			totalsLine(t, catalog.UnaffectedOnerous, "30"),
			totalsLine(t, catalog.Export, "40"),
			totalsLine(t, catalog.TaxedIVAP, "50"),
			checkedLine(t, calc.LineInput{Quantity: d("1"), UnitValue: d("20"), Affectation: catalog.TaxedIVAP, IGVPercent: d("5")}),
			totalsLine(t, catalog.TaxedFree, "60"),
			totalsLine(t, catalog.ExoneratedFree, "70"),
		}
		checkTotals(t, calc.TotalsInput{Lines: lines}, calc.TotalsResult{
			TaxedAmount: d("200"), ExoneratedAmount: d("20"), UnaffectedAmount: d("30"), ExportAmount: d("40"),
			IVAPBase: d("70"), IVAP: d("3"), IGV: d("28.5"), FreeAmount: d("130"), FreeIGV: d("10.8"),
			TotalTaxes: d("31.5"), LineExtension: d("360"), TaxInclusive: d("391.5"), Payable: d("391.5"),
		}, "")
	})
	t.Run("sum rounded taxes not whole document rate", func(t *testing.T) {
		line := totalsLine(t, catalog.TaxedOnerous, "0.027")
		checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line, line}}, calc.TotalsResult{
			TaxedAmount: d("0.06"), IGV: d("0.02"), TotalTaxes: d("0.02"),
			LineExtension: d("0.06"), TaxInclusive: d("0.08"), Payable: d("0.08"),
		}, "")
	})
	t.Run("ISC bases IGV IVAP other taxes and ICBPER", func(t *testing.T) {
		var lines []model.InvoiceLine
		for _, aff := range []catalog.IGVAffectation{catalog.TaxedOnerous, catalog.TaxedIVAP} {
			lines = append(lines, checkedLine(t, calc.LineInput{Quantity: d("2"), UnitValue: d("100"),
				Affectation: aff, ISCPercent: d("10"), OtherTaxPercent: d("5"), ICBPERFactor: d("0.5")}))
		}
		checkTotals(t, calc.TotalsInput{Lines: lines}, calc.TotalsResult{
			TaxedAmount: d("220"), IGV: d("39.6"), IVAPBase: d("220"), IVAP: d("8.8"), ISC: d("40"), ISCBase: d("400"),
			OtherTaxes: d("20"), OtherTaxesBase: d("400"), ICBPER: d("2"), TotalTaxes: d("110.4"),
			LineExtension: d("400"), TaxInclusive: d("510.4"), Payable: d("510.4"),
		}, "")
	})
	t.Run("base discount already applied and nonbase discount deducted once", func(t *testing.T) {
		var lines []model.InvoiceLine
		for _, reason := range []catalog.AllowanceChargeReason{catalog.DiscountItemTaxBase, catalog.DiscountItem} {
			lines = append(lines, checkedLine(t, calc.LineInput{Quantity: d("1"), UnitValue: d("100"),
				DiscountAmount: d("10"), DiscountReason: reason}))
		}
		checkTotals(t, calc.TotalsInput{Lines: lines, AllowanceTotal: d("5"), ChargeTotal: d("2"),
			PrepaidTotal: d("20"), Rounding: d("-0.01")}, calc.TotalsResult{
			TaxedAmount: d("190"), IGV: d("34.2"), TotalTaxes: d("34.2"), LineExtension: d("190"),
			TaxInclusive: d("224.2"), AllowanceTotal: d("15"), ChargeTotal: d("2"), PrepaidTotal: d("20"),
			Rounding: d("-0.01"), Payable: d("191.19"),
		}, "")
	})
	t.Run("gross prepayment exact zero and positive rounding", func(t *testing.T) {
		line := totalsLine(t, catalog.TaxedOnerous, "100")
		for _, rounding := range []string{"0", "0.01"} {
			checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line}, PrepaidTotal: d("118"), Rounding: d(rounding)},
				calc.TotalsResult{TaxedAmount: d("100"), IGV: d("18"), TotalTaxes: d("18"), LineExtension: d("100"),
					TaxInclusive: d("118"), PrepaidTotal: d("118"), Rounding: d(rounding), Payable: d(rounding)}, "")
		}
	})
}

func TestTotalsEveryAffectationAndLineBoundaries(t *testing.T) {
	for _, code := range []string{"10", "11", "12", "13", "14", "15", "16", "17", "20", "21", "30", "31", "32", "33", "34", "35", "36", "40"} {
		t.Run(code, func(t *testing.T) {
			line := totalsLine(t, catalog.IGVAffectation(code), "100")
			want := calc.TotalsResult{}
			switch code {
			case "10":
				want.TaxedAmount, want.IGV = d("100"), d("18")
			case "17":
				want.IVAPBase, want.IVAP = d("100"), d("4")
			case "20":
				want.ExoneratedAmount = d("100")
			case "30":
				want.UnaffectedAmount = d("100")
			case "40":
				want.ExportAmount = d("100")
			default:
				want.FreeAmount, want.FreeIGV = d("100"), line.IGV
			}
			if !line.Affectation.Free() {
				want.LineExtension = d("100")
				want.TotalTaxes = line.TotalTaxes
				want.TaxInclusive = want.LineExtension.Add(want.TotalTaxes)
				want.Payable = want.TaxInclusive
			}
			checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line}}, want, "")
		})
	}
	for _, in := range []calc.LineInput{
		{Quantity: d("1"), UnitValue: d("0.00000000001")},
		{Quantity: d("1"), UnitValue: d("100"), DiscountFactor: d("1")},
		{Quantity: d("2"), UnitPrice: d("3.54"), ISCFixedAmount: d("3")},
	} {
		line := checkedLine(t, in)
		checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line}}, calc.TotalsResult{
			TaxedAmount: line.TaxBase, IGV: line.IGV, ISC: line.ISC, ISCBase: line.ISCBase,
			TotalTaxes: line.TotalTaxes, LineExtension: line.LineAmount,
			TaxInclusive: line.LineAmount.Add(line.TotalTaxes), Payable: line.LineAmount.Add(line.TotalTaxes),
		}, "")
	}
}

func TestTotalsRejections(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(*calc.TotalsInput)
		message string
	}{
		{
			"nil lines", func(in *calc.TotalsInput) { in.Lines = nil },
			"calc: totals: at least one line is required",
		},
		{
			"empty lines", func(in *calc.TotalsInput) { in.Lines = []model.InvoiceLine{} },
			"calc: totals: at least one line is required",
		},
		{
			"unknown affectation", func(in *calc.TotalsInput) { in.Lines[0].Affectation = "99" },
			"calc: totals: line 1: catalog: unsupported IGV affectation \"99\"",
		},
		{
			"missing affectation", func(in *calc.TotalsInput) { in.Lines[0].Affectation = "" },
			"calc: totals: line 1: catalog: unsupported IGV affectation \"\"",
		},
		{
			"tax sum mismatch", func(in *calc.TotalsInput) { in.Lines[0].TotalTaxes = d("17.99") },
			"calc: totals: line 1: TotalTaxes must equal IGV + ISC + OtherTax + ICBPER",
		},
		{
			"line charge", func(in *calc.TotalsInput) { in.Lines[0].AllowanceCharges[0].Charge = true },
			"calc: totals: line 1: only discount reasons 00 and 01 are supported",
		},
		{
			"global base allocation",
			func(in *calc.TotalsInput) { in.Lines[0].AllowanceCharges[0].Reason = catalog.DiscountGlobalTaxBase },
			"calc: totals: line 1: only discount reasons 00 and 01 are supported",
		},
		{
			"unknown reason", func(in *calc.TotalsInput) { in.Lines[0].AllowanceCharges[0].Reason = "" },
			"calc: totals: line 1: only discount reasons 00 and 01 are supported",
		},
		{
			"discount exceeds base", func(in *calc.TotalsInput) { in.Lines[0].AllowanceCharges[0].Amount = d("101") },
			"calc: totals: line 1: discount exceeds its BaseAmount",
		},
		{
			"nonbase exceeds line",
			func(in *calc.TotalsInput) {
				in.Lines[0].AllowanceCharges[0].Reason = catalog.DiscountItem
				in.Lines[0].LineAmount = d("5")
			},
			"calc: totals: line 1: non-base discounts exceed LineAmount",
		},
		{
			"free with ancillary taxes",
			func(in *calc.TotalsInput) {
				in.Lines[0] = totalsLine(t, catalog.TaxedFree, "100")
				in.Lines[0].ISC = d("1")
				in.Lines[0].TotalTaxes = d("19")
			},
			"calc: totals: line 1: free operations cannot carry allowances, ISC, other taxes or ICBPER",
		},
		{
			"free with discount", func(in *calc.TotalsInput) { in.Lines[0].Affectation = catalog.TaxedFree },
			"calc: totals: line 1: free operations cannot carry allowances, ISC, other taxes or ICBPER",
		},
		{
			"zero tax scheme with IGV", func(in *calc.TotalsInput) { in.Lines[0].Affectation = catalog.ExoneratedOnerous },
			"calc: totals: line 1: affectation does not support IGV",
		},
		{
			"overdeduction", func(in *calc.TotalsInput) { in.AllowanceTotal = d("118") },
			"calc: totals: deductions exceed tax-inclusive amount plus charges",
		},
		{
			"rounding cannot mask overdeduction",
			func(in *calc.TotalsInput) {
				in.PrepaidTotal = d("118")
				in.Rounding = d("100")
			},
			"calc: totals: deductions exceed tax-inclusive amount plus charges",
		},
		{
			"rounding makes payable negative", func(in *calc.TotalsInput) { in.Rounding = d("-200") },
			"calc: totals: Payable cannot be negative",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			line := checkedLine(t, calc.LineInput{Quantity: d("1"), UnitValue: d("100"), DiscountAmount: d("10")})
			in := calc.TotalsInput{Lines: []model.InvoiceLine{line}}
			tt.change(&in)
			checkTotals(t, in, calc.TotalsResult{}, tt.message)
		})
	}
}

func TestTotalsMonetaryValidation(t *testing.T) {
	for _, field := range []string{"AllowanceTotal", "ChargeTotal", "PrepaidTotal", "Rounding"} {
		for _, value := range []string{"-0.01", "0.001"} {
			if field == "Rounding" && value == "-0.01" {
				continue
			}
			t.Run(field+value, func(t *testing.T) {
				in := calc.TotalsInput{Lines: []model.InvoiceLine{totalsLine(t, catalog.TaxedOnerous, "100")}}
				reflect.ValueOf(&in).Elem().FieldByName(field).Set(reflect.ValueOf(d(value)))
				message := "calc: totals: " + field + " must be cent-rounded"
				if field != "Rounding" {
					message += " and nonnegative"
				}
				checkTotals(t, in, calc.TotalsResult{}, message)
			})
		}
	}
	for _, field := range []string{"LineAmount", "TaxBase", "IGV", "TotalTaxes", "ISC", "ISCBase", "OtherTax", "OtherTaxBase", "ICBPER", "Amount", "BaseAmount"} {
		for _, value := range []string{"-0.01", "0.001"} {
			t.Run(field+value, func(t *testing.T) {
				line := checkedLine(t, calc.LineInput{Quantity: d("1"), UnitValue: d("100"), DiscountAmount: d("10")})
				target := reflect.ValueOf(&line).Elem()
				name := field
				if field == "Amount" || field == "BaseAmount" {
					target = reflect.ValueOf(&line.AllowanceCharges[0]).Elem()
					name = "discount " + field
				}
				target.FieldByName(field).Set(reflect.ValueOf(d(value)))
				checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line}}, calc.TotalsResult{}, "calc: totals: line 1: "+name+" must be cent-rounded and nonnegative")
			})
		}
	}
}

func TestTotalsNegativeUnitMoneyAndLateFailure(t *testing.T) {
	for _, field := range []string{"UnitValue", "UnitPrice", "ICBPERFactor"} {
		t.Run(field, func(t *testing.T) {
			line := totalsLine(t, catalog.TaxedOnerous, "100")
			reflect.ValueOf(&line).Elem().FieldByName(field).Set(reflect.ValueOf(d("-0.001")))
			checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line}}, calc.TotalsResult{},
				"calc: totals: line 1: "+field+" cannot be negative")
		})
	}
	t.Run("late invalid line discards earlier totals", func(t *testing.T) {
		first := totalsLine(t, catalog.TaxedOnerous, "100")
		second := totalsLine(t, catalog.TaxedIVAP, "100")
		second.TotalTaxes = d("5")
		checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{first, second}}, calc.TotalsResult{},
			"calc: totals: line 2: TotalTaxes must equal IGV + ISC + OtherTax + ICBPER")
	})
}

func TestTotalsManualTaxAmountsAreNotRecalculated(t *testing.T) {
	line := totalsLine(t, catalog.TaxedOnerous, "100")
	line.IGV, line.TotalTaxes = d("7"), d("7")
	checkTotals(t, calc.TotalsInput{Lines: []model.InvoiceLine{line}}, calc.TotalsResult{
		TaxedAmount: d("100"), IGV: d("7"), TotalTaxes: d("7"), LineExtension: d("100"),
		TaxInclusive: d("107"), Payable: d("107"),
	}, "")
}

func TestTotalsApplyEveryMonetaryField(t *testing.T) {
	r := calc.TotalsResult{}
	rv := reflect.ValueOf(&r).Elem()
	for i := 0; i < rv.NumField(); i++ {
		rv.Field(i).Set(reflect.ValueOf(d(fmt.Sprint(i + 1))))
	}
	inv := model.Invoice{Currency: "USD", Observation: "Keep"}
	got := r.Apply(inv)
	want := inv
	wv := reflect.ValueOf(&want).Elem()
	for i := 0; i < rv.NumField(); i++ {
		wv.FieldByName(rv.Type().Field(i).Name).Set(rv.Field(i))
	}
	assertFields(t, got, want)
	if !inv.Payable.IsZero() || inv.Currency != "USD" || inv.Observation != "Keep" {
		t.Fatal("Apply changed caller invoice")
	}
}

func TestTotalsApplyAndManualAmountsRemainExplicit(t *testing.T) {
	line := totalsLine(t, catalog.TaxedOnerous, "100")
	inv := model.Invoice{Type: catalog.Receipt, Series: "B001", Number: 7, Currency: "PEN",
		Lines: []model.InvoiceLine{line}, Payable: d("777"),
		Perception: &model.InvoicePerception{Amount: d("3")}, Detraction: &model.Detraction{Amount: d("12")},
		AllowanceCharges: []model.AllowanceCharge{{Reason: catalog.DiscountGlobal, Amount: d("2")}},
		Prepayments: []model.Prepayment{{Number: "F001-1", Total: d("10")}}}
	before := inv
	before.Lines = copyTotalsInput(calc.TotalsInput{Lines: inv.Lines}).Lines
	before.AllowanceCharges = append([]model.AllowanceCharge(nil), inv.AllowanceCharges...)
	before.Prepayments = append([]model.Prepayment(nil), inv.Prepayments...)
	perception, detraction := *inv.Perception, *inv.Detraction
	before.Perception, before.Detraction = &perception, &detraction
	r, err := calc.Totals(calc.TotalsInput{Lines: inv.Lines, AllowanceTotal: d("2"), PrepaidTotal: d("10")})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inv, before) {
		t.Fatal("Totals changed manual invoice amounts")
	}
	want := inv
	wv, rv := reflect.ValueOf(&want).Elem(), reflect.ValueOf(r)
	for i := 0; i < rv.NumField(); i++ {
		wv.FieldByName(rv.Type().Field(i).Name).Set(rv.Field(i))
	}
	got := r.Apply(inv)
	assertFields(t, got, want)
	if !reflect.DeepEqual(inv, before) {
		t.Fatal("Apply changed caller invoice")
	}
	if !got.Payable.Equal(d("106")) {
		t.Errorf("Payable = %s, want 106 without perception/detraction inference", got.Payable)
	}
}

func ExampleTotals() {
	line, err := calc.Line(calc.LineInput{
		Description: "Service", Quantity: d("2"), UnitValue: d("100"),
		DiscountReason: catalog.DiscountItem, DiscountAmount: d("10"),
	})
	if err != nil {
		panic(err)
	}
	in := model.Invoice{
		Type: catalog.Invoice, Series: "F001", Number: 1, Currency: "PEN",
		Lines: []model.InvoiceLine{line},
		AllowanceCharges: []model.AllowanceCharge{
			{Reason: catalog.DiscountGlobal, Amount: d("5"), BaseAmount: d("200")},
			{Charge: true, Reason: catalog.ChargeGlobal, Amount: d("2"), BaseAmount: d("200")},
		},
		Prepayments: []model.Prepayment{{DocType: "02", Number: "F001-99", Total: d("36")}},
	}
	totals, err := calc.Totals(calc.TotalsInput{
		Lines: in.Lines, AllowanceTotal: d("5"), ChargeTotal: d("2"), PrepaidTotal: d("36"),
	})
	if err != nil {
		panic(err)
	}
	inv := totals.Apply(in)
	fmt.Println(inv.LineExtension, inv.TotalTaxes, inv.Payable)
	// Output: 200 36 187
}
