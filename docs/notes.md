# Credit and debit notes

Use `model.Note` to modify an issued factura (`catalog.Invoice`, `01`) or
boleta (`catalog.Receipt`, `03`). HTTP and MCP interfaces are outside this
library's scope.

## Choose the document and reason

| Note type | Reason catalog | Example reason |
| --- | --- | --- |
| `catalog.CreditNote` (`07`) | 09 | `string(catalog.CreditCancel)` |
| `catalog.DebitNote` (`08`) | 10 | `string(catalog.DebitInterest)` |

Set `Series`, `Number`, `IssueDate`, `Currency`, `Company` and `Client` for the
note itself. Set `AffectedType` and `AffectedNumber` for the original document
(for example, `catalog.Invoice` and `"F001-45"`). `Reason` is a catalog code;
`Description` explains the adjustment in text. Lines reuse `model.InvoiceLine`.
Supply the note's own amounts, taxes and legends, not the original document's
amounts unless the adjustment calls for them. Rendering does not calculate or
reconcile totals.

`Note.ID()` returns `Series-Number`, such as `FC01-3`.
`Note.FileName()` returns `RUC-Type-Series-Number`, without an extension, such
as `20000000001-07-FC01-3`.

## Render, build or send

- `ubl.Note(note)` produces unsigned UBL 2.1 XML. Credit notes use `CreditNote`,
  `CreditNoteLine`, `CreditedQuantity` and `LegalMonetaryTotal`; debit notes use
  `DebitNote`, `DebitNoteLine`, `DebitedQuantity` and `RequestedMonetaryTotal`.
  Both carry the original document in `BillingReference` and the adjustment
  reason in `DiscrepancyResponse`.
- `client.BuildNote(note)` renders and signs using the injected signer. It does
  not call the transport. The signature occupies `ext:ExtensionContent` and
  is referenced by the UBL signature URI.
- `client.SendNote(ctx, note)` builds, signs, calls the injected transport's
  `SendBill` with the note's filename, and parses the zipped CDR.

Store `result.XML` whenever available, including when `SendNote` returns an
error. It is the signed XML sent to transport. Rendering or signing failure
returns no XML and does not call transport. Transport or CDR parse failure
retains the signed XML. Wrapped errors preserve their original type and can
be inspected with `errors.Is` or `errors.As` (including `*transport.Error`).

A rejected CDR is a response, not a Go error. Check `result.CDR.Accepted()` and
keep `result.CDR.XML` as well as the signed note. A CDR with observations can
still be accepted.

## Validation and proof boundaries

Existing validation checks the note type and its reason catalog, nonempty
series and currency, a positive number, affected type `01` or `03`, an affected
number containing a hyphen, a nonblank description, at least one line, and
supported line IGV affectations. These are limited structural checks, not a
complete SUNAT business-rule validator. They do not verify the original
comprobante exists or enforce every identifier or series format. Payment terms
are optional; unlike facturas, notes do not default to `Contado`.

Offline tests cover both note types through `ubl.Note`, `BuildNote` and
`SendNote`, including accepted, observed and rejected synthetic CDRs, pipeline
failures, context propagation, error identity and XML retention. Signing tests
use locally generated test certificates. When `xmlsec1` is installed, they
independently verify both signed note roots and reject changes to the reference
or payable amount; otherwise that integration test is explicitly skipped, as
in the invoice tests. These tests do not prove XSD compliance, SUNAT beta or
production acceptance, or authorization to issue real documents. No external
service is called by the note tests.
