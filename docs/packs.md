# Submit a caller-prepared pack and keep its raw response archive

`(*transport.SOAPClient).SendPack(ctx, fileName, zipContent)` sends a ZIP of
signed XML documents and returns a ticket. This is a transport-only operation:
there is no `Client.SendPack`, pack builder, or multi-CDR result facade.

## Quick path

1. Produce signed XML with the existing `Client.BuildInvoice` / `Client.BuildNote`
   methods. Supply and reconcile document amounts yourself.
2. Assemble the eligible signed XML files into a ZIP in caller code (for example,
   using Go's `archive/zip`). Preserve each document's required filename.
3. Submit that ZIP, then poll **billService** with `SOAPClient.GetStatus`.

Given an explicitly selected endpoint, SOL credentials, a context and the
caller-prepared `zipContent`:

```go
soap := transport.New(endpoint, creds)
ticket, err := soap.SendPack(ctx, "20100066603-LT-20160822-1.zip", zipContent)
if err != nil {
    return err
}
// Persist ticket and the submitted ZIP. Poll later using your retry policy.
status, err := soap.GetStatus(ctx, ticket)
if err != nil {
    return err
}
if status.InProcess() { // 98: schedule another poll, not a successful issuance.
    return nil
}
// Preserve status.Code and status.Content (raw response ZIP) when present.
// Caller inspects all per-document CDRs and the processing report.
// A finished ticket alone does not establish acceptance of every document.
```

**Do not use `Client.Status` for sendPack tickets.** That facade parses a single
CDR and does not aggregate per-document results or interpret a pack report.
`SOAPClient.GetStatus` instead returns `transport.Status.Content` unchanged
following base64 decoding, allowing the caller to handle the complete archive.

## Exact transport contract

```go
type PackTransport interface {
    SendPack(ctx context.Context, fileName string, zipContent []byte) (ticket string, err error)
}
```

`PackTransport` is an optional capability implemented by `*SOAPClient`, independent
of `BatchTransport`. Existing `Transport`, `BatchTransport`, `ConsultTransport`
and `CPEStatusTransport` implementations need no new methods. Polling remains the
existing `SOAPClient.GetStatus(ctx, ticket)` operation.

| Detail | Behavior |
| --- | --- |
| Request | `sendPack` in `http://service.sunat.gob.pe`, SOAPAction `urn:sendPack` |
| `fileName` | Complete ZIP filename including `.zip`; XML-escaped, no extension added |
| `contentFile` | Base64 of exactly the supplied ZIP bytes; no recompression or mutation |
| Ticket | Trimmed response text; missing, empty or whitespace-only ticket is an error |
| Polling | `getStatus(ticket)` on billService; `98` pending, `0` processed, `99` processed with errors |
| Poll content | Raw response ZIP, potentially multiple per-document CDRs and a report; may be absent |

Unlike `SendSummary`, which accepts signed XML and creates a single-entry ZIP,
`SendPack` accepts an already prepared archive. It does not inspect ZIP contents,
validate signatures, select documents, derive totals, or enforce business rules.
The caller owns document eligibility, archive naming and limits under the
applicable SUNAT contract. The documented operation is an optional direct SEE
taxpayer capability, **not OSE-only**; its presence in the WSDL is not proof that
a particular taxpayer, endpoint or payload is authorized.

Requests reuse the existing WS-Security SOL authentication, HTTP client, context
and SOAP fault handling. `errors.As` retains `*transport.Error` and
`*transport.HTTPError`; `errors.Is` retains context cancellation/deadline errors.
Non-SOAP HTTP response bodies are not included in errors. Polling rejects missing
status/statusCode and invalid base64 content; decoding errors retain their
identity. A blank polling ticket is rejected locally. No automatic polling,
retry, persistence or per-document acceptance decision is provided.

## Contract evidence and proof limits

Official references supplied for this implementation:

- [SUNAT Manual del Programador, May 2021, pp. 12–20](https://cpe.sunat.gob.pe/sites/default/files/inline-files/manual_programador%20%281%29.pdf).
- [Production billService WSDL](https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService?wsdl)
  and [request/response schema](https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService.xsd2.xsd):
  `fileName` plus base64Binary `contentFile`, returning `ticket`.

Offline `transport/pack_test.go` uses in-memory ZIPs and local HTTP servers to
check operation, namespace, action, authentication, escaped/exact filename,
byte-for-byte ZIP transmission, tickets, faults, HTTP/context errors, raw
multi-entry response archives and invalid base64. It also pins unchanged
`SendSummary` ZIP behavior and pending `GetStatus` semantics.

No live sendPack submission or external attachment/MTOM compatibility test was
performed. This implementation uses inline base64 SOAP content; offline tests
are not a claim of live attachment compatibility, SUNAT acceptance, taxpayer
authorization or production readiness. No production calls are needed to run
`go test ./transport -count=1`.
