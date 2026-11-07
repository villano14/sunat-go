# Query document status without recovering a CDR

Use `Client.QueryStatus` for a document's status on **billConsultService**.
Use `Client.QueryCDR` to recover its CDR, and `Client.Status(ticket)` to poll an
asynchronous summary or voided-document submission. These are distinct flows,
even though both services name an operation `getStatus`.

## Quick path

Given a configured `*sunat.Client` and a context:

```go
result, err := client.QueryStatus(ctx, "20000000001", catalog.Invoice, "F022", 7782)
if err != nil {
    return err
}
// Preserve result.StatusCode and result.Message, including unknown codes.
if result.Accepted() {
    // SUNAT reports document status 0001. This result is not a CDR.
}
```

| Method | Service / SOAP operation | Result |
| --- | --- | --- |
| `QueryStatus` | billConsultService / `getStatus` | Status code and message only |
| `QueryCDR` | billConsultService / `getStatusCdr` | Status plus optional parsed/zipped CDR |
| `Status(ticket)` | billService / `getStatus` | Ticket state plus optional parsed/zipped CDR |

The low-level status-only API is `SOAPClient.GetStatusCpe`, returning
`transport.CPEStatus`. Custom transports opt in through
`transport.CPEStatusTransport`; existing `Transport`, `BatchTransport`, and
`ConsultTransport` interfaces have not changed. An unsupported transport causes
`QueryStatus` to wrap `sunat.ErrStatusConsultUnsupported`.

## Interpret document codes exactly

| `StatusCode` | Meaning | Predicate |
| --- | --- | --- |
| `0001` | Exists and accepted | `Accepted()` |
| `0002` | Rejected | `Rejected()` |
| `0003` | Voided | `Voided()` |
| `0011` | Does not exist | `NotFound()` |
| Other | Preserved, not inferred | All predicates false |

Codes remain strings, including their leading zeros. Neither `0` nor `1` is
inferred to mean `0001`. Rejected, voided, absent, and unknown codes are results,
not transport errors. Status-only queries ignore optional response content:
they do not decode base64, parse a CDR, or infer acceptance from its presence.
A zero result, including a result returned with an error, is not accepted.

## Endpoints, validation, and errors

SUNAT documents this consult flow for Factura (`01`) and F-series credit/debit
notes (`07`, `08`). Boleta summary processing uses its batch ticket instead.
There is no published beta consult endpoint. `SOAPClient` defaults to
`transport.ConsultProduction` only for `BillProduction`; otherwise consult
methods return `transport.ErrNoConsultEndpoint`. Tests or alternate deployments
can explicitly set `transport.WithConsultEndpoint`.

Consult requests reuse SOL WS-Security authentication, HTTP/context handling,
and SOAP fault mapping. The number is validated as `1..2147483647`, matching the
existing CDR query convention and SUNAT's `xsd:int` field. No additional local
RUC, document-type, or series restrictions were introduced. Missing status or
statusCode is an error. Wrapping preserves `errors.Is` / `errors.As` identities
for context errors, missing endpoints, `*transport.Error`, and
`*transport.HTTPError`; HTTP response bodies are not included in errors.

## Evidence and limits

Contract: [SUNAT Manual del Programador, May 2021, Annex 2 pp. 52–54](https://cpe.sunat.gob.pe/sites/default/files/inline-files/manual_programador%20%281%29.pdf)
and the [official consult WSDL](https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService?wsdl)
([response schema](https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService?xsd=1)).
Offline tests use `httptest` to check request fields, escaping, authentication,
endpoint separation, errors, and compatibility. They do not prove production
availability, credential authorization, or acceptance of a real document.
