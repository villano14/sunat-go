# Individual boletas through the invoice API

An individual boleta (`03`) is supported by the library's existing invoice
pipeline: use `model.Invoice` with `Type: catalog.Receipt`, render/sign it and
submit through the injected transport's `SendBill`. This describes an
implemented mechanism, **not universal endpoint suitability or real SUNAT
acceptance** for a taxpayer, transaction or configuration.

## Quick path: unsigned, offline example

From the module root:

```sh
go run ./examples/receipt
```

The [example](../examples/receipt/main.go) prepares one line with `calc.Line`,
explicitly calls `calc.Totals` and `TotalsResult.Apply`, then prints unsigned
UBL XML with `ubl.Invoice`. It uses synthetic issuer/consumer data, series
`B001`, a DNI identity, and a payable amount of `236.00` PEN. It reads no
certificates or credentials and makes no network requests. The XML is not a
signed or issued comprobante and is not acceptance evidence.

## Render, build or send

| Public API | Effect for `catalog.Receipt` |
|---|---|
| `ubl.Invoice(inv)` | Unsigned UBL 2.1 `Invoice` root, `InvoiceTypeCode` `03`, document ID such as `B001-1` |
| `client.BuildInvoice(inv)` | Same rendering, signed by the injected signer; no transport call |
| `client.SendInvoice(ctx, inv)` | Builds/signs, calls `SendBill` with context and filename such as `20000000001-03-B001-1` (no extension), parses the returned zipped CDR |

Choose the SOAP endpoint explicitly when constructing the transport; the
library does not select a different route for boletas or establish that the
configured endpoint is appropriate for your issuer and operation. Offline
coverage and the prior factura beta result do not establish individual boleta
acceptance in beta or production. No live individual-boleta check is claimed.
Series, document identifiers, consumer data and eligibility remain caller
responsibilities; the example is not a new business validation rule.

Supply amounts manually or opt into [line calculation and document
aggregation](calculations.md). Render/build/send preserve manual amounts and
never call `calc.Totals` automatically. Supported aggregation sums rounded
line taxes/bases and explicit non-base adjustments; global tax-base allocation,
rate applicability, perception/detraction eligibility and other excluded
policies still belong to the caller. Structural checks and field rendering are
not comprehensive SUNAT business validation.

## The CDR, not transport success, determines the outcome

Given an already configured client and invoice:

```go
result, err := client.SendInvoice(ctx, inv)
// Persist result.XML whenever non-empty, including on errors.
if err != nil {
    return err // errors.Is/As retain transport and parse error identity.
}
// Persist result.CDR.XML and inspect Code, Description and Notes.
if !result.CDR.Accepted() {
    return fmt.Errorf("boleta rejected: %s: %s", result.CDR.Code, result.CDR.Description)
}
```

A rejected CDR is returned as a response, not a Go error. The parser treats code
`0` as accepted and codes `4000+` as accepted with observations; inspect notes
and descriptions rather than discarding them. A successful HTTP/SOAP exchange
alone is not document acceptance. A transport or CDR parse error retains the
signed `result.XML`, but provides no accepted CDR. A render/sign failure stops
before transport and returns no XML.

## Summaries, annulments and notes remain separate

Individual `sendBill` support does not remove the RC summary flow for applicable
reporting or annulments, or the RA voided-document flow where applicable. Use
the appropriate procedure for the document, issuer and current rules; do not
assume that individual submission replaces every reporting obligation. See
[summaries](summaries.md) for the library's RC API and evidence limits.
[Credit/debit notes](notes.md) express supported adjustments to referenced
documents; their reason codes are not interchangeable with arbitrary
cancellation or a blanket substitute for summary/annulment procedures.

## Evidence and next boundary

Offline public-API tests pin boleta identity/namespaces, manual amounts,
BuildInvoice signing, SendBill filename/context, accepted/observed/rejected
synthetic CDRs and signed XML retention on errors. Real signing uses generated
test certificates; `xmlsec1`, when installed, independently verifies the signed
boleta and rejects ID/amount tampering. That check explicitly skips if absent.
The existing factura `01` tests and golden remain unchanged. None of these
checks proves live issuance, comprehensive XSD/business compliance or production
certification; the unsigned example is separate from signed test evidence.

The [roadmap](roadmap.md) retains the dated RC beta HTTP `401` and missing GRE /
production evidence. No service retries or production submissions are implied.
HTTP serving, PDF generation, persistence and MCP stay outside this module.
A future application API should consume this library for calculation, XML,
signing, transport and CDR handling; build MCP on that application afterward.
