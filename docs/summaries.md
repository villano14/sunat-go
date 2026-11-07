# Include an optional perception in a daily summary

Set `model.SummaryLine.Perception` to include the perception reference for that
line. Leave it `nil` to keep the existing summary XML byte-for-byte unchanged.
This is not the standalone Comprobante de Percepción (`40`) flow.

## Quick path

For an existing `model.Summary` with at least one line:

```go
summary.Lines[0].Perception = &model.SummaryPerception{
    Regime:         catalog.PerceptionInternalSale, // 01
    Rate:           decimal.RequireFromString("2"),
    Amount:         decimal.RequireFromString("4"),
    TotalCollected: decimal.RequireFromString("204"),
    TaxableAmount:  decimal.RequireFromString("200"),
}
xml, err := ubl.Summary(summary)
```

`Client.BuildSummary` signs the same rendering; `Client.SendSummary` sends it
and returns a ticket to poll with `Client.Status`.

## XML mapping

Each `sac:SummaryDocumentsLine` has zero or one
`sac:SUNATPerceptionSummaryDocumentReference`, after `cac:BillingReference`
(when present) and before `cac:Status`, followed by `sac:TotalAmount`. Its
children are emitted in this order, with no ID or date:

| Field | XML child | Example |
| --- | --- | --- |
| `Regime` | `sac:SUNATPerceptionSystemCode` | `01` |
| `Rate` | `sac:SUNATPerceptionPercent` | `2.00` |
| `Amount` | `cbc:TotalInvoiceAmount` | `4.00` |
| `TotalCollected` | `sac:SUNATTotalCashed` | `204.00` |
| `TaxableAmount` | `cbc:TaxableAmount` | `200.00` |

Despite its name, `TotalInvoiceAmount` here carries the **perception amount**,
not the document's total. `TotalCollected` includes perception;
`TaxableAmount` is the perception base.

The three monetary children use the same `currencyID` as the rest of the line:
`Summary.Currency`, defaulting to `PEN`. All decimal fields render with two
decimal places using the existing amount formatter. A non-nil reference emits
all five children, even for zero amounts or rate. Only `nil` omits the block.

## Validation and caller ownership

The regime is checked using existing catalog 22 validation
(`catalog.PerceptionRate`). The caller supplies the rate and all amounts;
rendering does not substitute a default rate, derive totals, reconcile them
against the line total, or mutate caller input. Catalog validation is not a
claim that every catalog regime applies to every transaction. No additional
summary-specific business restrictions are invented here.

## Evidence and limits

Contract: [SUNAT Guía de elaboración del Resumen Diario de Boletas de Venta,
January 2018 v2, pp. 9, 13, 20–22](https://cpe.sunat.gob.pe/sites/default/files/inline-files/GUIA_Resumen_de_Boletas_11-01-2018%20%282%29_2_0%20%281%29_0.pdf).
Tests parse XML to check namespaces, cardinality, ordering, amounts, currency,
line isolation, validation, and unchanged caller input. The existing golden
summary is retained without changes.

A local, git-ignored schema cache is available under `testdata-xsd/` in this
checkout; it is not guaranteed to exist in a fresh clone. The repository's
`scripts/fetch-xsd.sh` describes its sources: the UBL 2.0 SUNAT schema copies
are distributed by [Greenter's ubl-validator](https://github.com/thegreenter/ubl-validator),
with default revision `980bad9412eab51516e4879b06d8d2e723c62f0d` (overridable by
`UBL_VALIDATOR_REF`). The script keeps existing files, so that default alone
does not attest the revision of an already populated cache.

`TestSummaryPerceptionXSD` renders through the public `ubl.Summary` API and
runs `xmllint --nonet --noout --schema` against the cached
`testdata-xsd/2.0/maindoc/UBL-SummaryDocuments-2.0.xsd`. Its imported
`2.0/common/UBLPE-SunatAggregateComponents-1.0.xsd` requires the optional
perception reference before Status (lines 398 and 413). Both the unchanged
nil-perception golden output and generated perception summaries, with and
without BillingReference, pass this schema check. The test explicitly skips
when `xmllint` or a required cache file is absent; it never downloads schemas.

This proves schema conformance for the tested generated documents against the
available cached schema set, not SUNAT's live business-rule validation or
production acceptance. No production submission was made.
