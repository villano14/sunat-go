# Calculate lines and optional document totals

`calc.Line(calc.LineInput)` returns a `model.InvoiceLine` with decimal prices,
amounts, allowances and taxes for invoices, receipts and notes.
`calc.Totals(calc.TotalsInput)` optionally aggregates those lines and explicit
non-tax-base adjustments. Both are pure arithmetic helpers, not tax-policy or
complete document validators. Existing manual-amount APIs remain unchanged.

## Quick path

Use decimal strings, not binary floating-point money, and supply exactly one
positive `UnitValue` or `UnitPrice` plus a positive `Quantity`:

```go
import (
    "github.com/shopspring/decimal"
    "github.com/villano14/sunat-go/calc"
)

line, err := calc.Line(calc.LineInput{
    Description: "Service",
    Quantity:    decimal.RequireFromString("2"),
    UnitPrice:   decimal.RequireFromString("118"),
})
if err != nil {
    return err
}
// line.UnitValue = 100; line.UnitPrice = 118
// line.LineAmount = 200; line.TaxBase = 200
// line.IGV = 36; line.TotalTaxes = 36
// Assign line to your document's Lines; this does not compute document totals.
```

`Unit` defaults to `NIU`, `Affectation` to `catalog.TaxedOnerous`, and `PriceType`
to `catalog.UnitPriceWithTax` (`01`). Product identifiers, description, explicit
unit and item attributes pass through. Attributes are not deep-copied: do not
treat the returned slice as independent mutable storage.

## Prices, rounding and affectations

All amounts use `shopspring/decimal`. Rounding is half away from zero.

| Figure | Calculation |
|---|---|
| Unit value / price | Round to at most 10 decimal places, including supplied retail prices |
| Gross line value | Round `Quantity * rounded UnitValue` to 2 places |
| Line amount | Gross less a base-reducing line discount, if any |
| IGV base | Line amount plus rounded ISC |
| IGV | Round `TaxBase * IGVPercent / 100` to 2 places, using the rounded base |
| Total taxes | Sum line IGV, ISC, other tax and ICBPER |

For example, a unit value of `0.027` at quantity `1` produces a base of `0.03`
and IGV of `0.01`, not IGV calculated directly from the unrounded product.
A positive input can round to zero; the helper does not impose a minimum
positive result.

`UnitPrice` includes IGV (or IVAP) and ISC. If given a unit value, the helper adds
ISC per unit before applying the IGV rate. If given a retail price, it removes
IGV and then ISC to recover the unit value. Other taxes and ICBPER are calculated
separately; they are **not included in or removed from `UnitPrice`** by this API.
Prices do not change when a line discount is applied.

| Affectation | Rate and output |
|---|---|
| Taxed onerous (`10`) | Default 18%; a nonzero `IGVPercent` overrides it |
| IVAP (`17`) | Default 4%; a nonzero rate overrides it; the line uses `IGVPercent` and `IGV` fields |
| Exonerated (`20`), unaffected (`30`), export (`40`) | Zero IGV; any nonnegative supplied rate is ignored |
| Taxed free (`11`–`16`) | Default 18% or supplied nonzero rate; notional IGV |
| Other free (`21`, `31`–`36`) | Zero IGV; any nonnegative supplied rate is ignored |

For free operations, `UnitValue` is zero and `UnitPrice` holds the **reference
unit value without IGV**, with `PriceType = catalog.ReferenceValue` (`02`).
`LineAmount` and `TaxBase` hold the reference line amount, and `TotalTaxes`
contains any notional IGV. Those figures are not amounts to collect. `Totals`
keeps them separate from payable operations in `FreeAmount` and `FreeIGV`.
A free reference supplied via `UnitPrice` is stripped of the applicable IGV;
supply `UnitValue` when the reference is already tax-exclusive.

## Discounts and additional taxes

Set at most one of `DiscountFactor` (e.g. `0.10`, not `10`) and `DiscountAmount`
(tax-exclusive). The resulting `AllowanceCharges` entry has the original rounded
gross as `BaseAmount`. An amount is rounded to cents and its factor is derived
to 5 decimal places; a supplied factor is preserved and its amount rounded to
cents. Full discounts are allowed. Limits on amount discounts are checked after
rounding to cents.

| Setting | Effect |
|---|---|
| `DiscountReason = catalog.DiscountItemTaxBase` (`00`, default) | Reduces line amount before ISC, IGV and other percentage taxes |
| `DiscountReason = catalog.DiscountItem` (`01`) | Emits an allowance but leaves line amount and taxes unchanged; `Totals` deducts it once via `AllowanceTotal`, or the manual caller must do so |
| `ISCPercent` | System `01`, `catalog.ISCFixedAmount` despite that constant's name; round `LineAmount * percent / 100` to cents |
| `ISCFixedAmount` | System `02`, `catalog.ISCMinimum`; round `Quantity * fixed amount` to cents, unaffected by discounts |
| `OtherTaxPercent` | Round `LineAmount * percent / 100` to cents; exposes base and rate for scheme `9999` |
| `ICBPERFactor` | Round `Quantity * factor` to cents; independent of the line value |

ISC system is inferred if omitted; an explicit system must match the supplied
percent or fixed amount. Set at most one ISC calculation mode. System `03`
(`catalog.ISCRetail`) is unsupported and is rejected even with no ISC amount;
its fields must be prepared manually outside `Line`.

## Rejections and scope limits

Errors return a zero `model.InvoiceLine` and leave the input unchanged. The
helper rejects:

- Non-positive quantity, missing/both prices, negative prices or negative IGV.
- Unsupported affectations; conflicting discount modes; discount reasons other
  than `00`/`01`; negative discounts, factors above one, discounts on a rounded
  zero-value line, or a rounded discount exceeding the gross line value.
- Negative/conflicting ISC inputs, mismatched ISC systems, unsupported system
  `03`, or a retail price that cannot cover fixed ISC after removing IGV.
- Negative other-tax rates or ICBPER factors.
- Any discount, ISC, other tax or ICBPER on a free operation.

This is an arithmetic helper, not complete document or tax-policy validation.
It does not choose legally applicable rates, validate product identifiers or
issuer/customer data, enforce tax applicability, or calculate amounts in words.

## Opt-in document aggregation

```go
func Totals(in calc.TotalsInput) (calc.TotalsResult, error)
func (r calc.TotalsResult) Apply(inv model.Invoice) model.Invoice
```

`TotalsInput` has `Lines []model.InvoiceLine` and four decimal fields:
`AllowanceTotal` (additional document non-base discounts), `ChargeTotal`
(non-base charges), `PrepaidTotal` (gross advances including tax), and signed
`Rounding`. All amounts are in one caller-chosen currency. There is no input for
base-changing global discounts/charges or their allocation among tax categories.
Do not pass such adjustments disguised as non-base amounts.

`TotalsResult` has all 22 monetary fields of `model.Invoice`. These are the
aggregation rules; taxes are **summed, never recomputed at a document rate**:

| Result | Source |
|---|---|
| `TaxedAmount`, `ExoneratedAmount`, `UnaffectedAmount`, `ExportAmount` | Sum each paid category's `TaxBase`, classified with `catalog.SchemeFor` |
| `IVAPBase`, `IVAP` | Sum affectation `17`'s `TaxBase` and line `IGV`; excluded from ordinary `IGV`/`TaxedAmount` |
| `FreeAmount`, `FreeIGV` | Sum free reference `TaxBase` and notional `IGV`; excluded from all collectible totals |
| `IGV` | Sum ordinary paid IGV, excluding IVAP and free notional tax |
| `ISC`, `ISCBase` | Sum paid line ISC and its supplied base |
| `OtherTaxes`, `OtherTaxesBase`, `ICBPER` | Sum paid line `OtherTax`, `OtherTaxBase`, `ICBPER` |
| `TotalTaxes` | Sum paid line `TotalTaxes` = IGV + IVAP + ISC + other taxes + ICBPER |
| `LineExtension` | Sum paid `LineAmount`; free reference values are excluded |
| `TaxInclusive` | `LineExtension + TotalTaxes`, before non-base adjustments and advances |
| `AllowanceTotal` | Input document allowance + all line reason `01` amounts; reason `00` is already in line amounts and is not deducted again |
| `ChargeTotal`, `PrepaidTotal`, `Rounding` | Explicit input, unchanged |
| `Payable` | `TaxInclusive - AllowanceTotal + ChargeTotal - PrepaidTotal + Rounding` |

`TaxBase` includes ISC when prepared by `Line`, so `TaxedAmount` can differ from
`LineExtension`. For example, a paid value of 100 with 10 ISC yields an IGV base
of 110, not 100. Two separate lines with rounded bases 0.03 and IGV 0.01 each
produce aggregate IGV 0.02, **not** round(0.06 × 18%) = 0.01. Mixed rates and
mixed categories require no inferred common rate.

### Explicit invoice assembly

This example also supplies the document records matching its explicit totals;
`Apply` does not synthesize those records. Company and client are caller-owned.

```go
import (
    "time"

    "github.com/shopspring/decimal"
    "github.com/villano14/sunat-go/calc"
    "github.com/villano14/sunat-go/catalog"
    "github.com/villano14/sunat-go/model"
)

func assembleInvoice(company model.Company, client model.Client) (model.Invoice, error) {
    money := decimal.RequireFromString
    line, err := calc.Line(calc.LineInput{
        Description: "Service", Quantity: money("2"), UnitValue: money("100"),
        DiscountReason: catalog.DiscountItem, DiscountAmount: money("10"),
    })
    if err != nil {
        return model.Invoice{}, err
    }
    inv := model.Invoice{
        Type: catalog.Invoice, Series: "F001", Number: 1,
        IssueDate: time.Now(), Currency: "PEN", Company: company, Client: client,
        Lines: []model.InvoiceLine{line},
        AllowanceCharges: []model.AllowanceCharge{
            {Reason: catalog.DiscountGlobal, Amount: money("5"), BaseAmount: money("200")},
            {Charge: true, Reason: catalog.ChargeGlobal, Amount: money("2"), BaseAmount: money("200")},
        },
        Prepayments: []model.Prepayment{{DocType: "02", Number: "F001-99", Total: money("36")}},
    }
    totals, err := calc.Totals(calc.TotalsInput{
        Lines: inv.Lines, AllowanceTotal: money("5"), ChargeTotal: money("2"),
        PrepaidTotal: money("36"),
    })
    if err != nil {
        return model.Invoice{}, err
    }
    inv = totals.Apply(inv)
    // LineExtension=200, IGV=36, TaxInclusive=236, AllowanceTotal=15.
    // Payable=236-15+2-36=187. Line discount 10 is NOT supplied again.
    return inv, nil
}
```

For a boleta set `Type: catalog.Receipt` and a suitable `B...` series. The
arithmetic is unchanged. Supplying a valid identity, issue details, legends,
payment terms and applicable tax policy remains the caller's responsibility.

### Aggregate rejections and guarantees

Errors return an exactly zero `TotalsResult`; neither success nor rejection
changes the caller's invoice, lines, allowance entries or item attributes.
`Totals` rejects:

- Empty lines or any unsupported/missing affectation (unlike `Line`, no default
  affectation is inferred for an already prepared line).
- Negative or non-cent-rounded line `LineAmount`, `TaxBase`, `IGV`, `TotalTaxes`,
  `ISC`, `ISCBase`, `OtherTax`, `OtherTaxBase`, `ICBPER`, or discount `Amount` and
  `BaseAmount`. Input document adjustments must also be cent-rounded and
  nonnegative, except `Rounding`, which may have either sign. Numerically equal
  representations such as `1.000` are accepted; sub-cent values are not silently
  normalized. Unit values/prices and ICBPER per-unit factors may retain their
  precision, but cannot be negative.
- A line `TotalTaxes` unequal to `IGV + ISC + OtherTax + ICBPER`. For IVAP,
  `IGV` is the line's IVAP amount as produced by `Line`; for free lines it is
  notional IGV. All supported free and IVAP `Line` outputs remain accepted.
- Any line charge or discount reason other than `00`/`01`, discounts exceeding
  their supplied base, or summed reason `01` discounts exceeding `LineAmount`.
- Free lines with allowances, ISC/base, other tax/base or ICBPER; nonzero IGV on
  affectations where `Line` always produces zero IGV.
- Allowances plus prepayments exceeding `TaxInclusive + ChargeTotal` before
  rounding, or a negative final `Payable`. Positive rounding cannot conceal
  overdeductions. Zero payable, full discounts and rounded-zero lines are valid.

### Manual compatibility and exclusions

`Line` behavior is unchanged. Calling `Totals` is optional; model, UBL rendering,
building and sending never call it implicitly or replace your manually supplied
amounts. `Apply` returns an invoice value with only its monetary fields replaced;
it does not validate the result or deep-copy existing slices/pointers. You can
instead assign individual result fields yourself, including to note models.

The helper trusts already calculated tax amounts and bases: it does not check
quantity × price, infer or enforce rates/eligibility, reconcile allowance
factors, or recalculate tax bases. Its stricter aggregation checks do not change
what the existing manual-amount APIs accept. It does not create or reconcile
document allowance/prepayment records, change payment terms, or produce legends
or amounts in words.

**Excluded policy:** global base-changing allocations and advance tax-base
allocations; perception, detraction and retention calculations; currency
conversion; installment scheduling; rate and legal applicability decisions.
Perception/detraction remain explicit `model.Invoice`/caller policy and are not
added to or subtracted from `Payable`. The explicit gross `PrepaidTotal` is only
a payable deduction, not an automatic adjustment of IGV or any category base.
Documents requiring excluded policies can still be prepared manually.

## Offline verification

Run `go test ./calc -count=1` for deterministic public-API tests of prices,
rounding, all supported affectations, discounts, ISC, other taxes, ICBPER,
metadata and validation with unchanged inputs. It also covers all aggregate
monetary fields, mixed categories/rates, free/IVAP segregation, per-line rounding,
adjustments, zero results on errors, pure application and manual compatibility.
Run `go test ./...` for the repository suite.

These tests do not contact SUNAT or prove official rate applicability, live
service acceptance, successful issuance, or any absent document aggregation
policy. No production/external verification is claimed.
