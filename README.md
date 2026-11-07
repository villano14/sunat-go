# sunat-go

Go library for Peruvian electronic invoicing (SUNAT): build UBL XML, sign it with
XMLDSig, send it to SUNAT web services, and read CDR responses.

Inspired by [Greenter](https://github.com/thegreenter/greenter) (PHP), designed as an
idiomatic Go library: small packages, no framework, no database.

> Implemented document flows have offline tests; this is not a claim that the
> library is complete or every flow is accepted by SUNAT. Optional document
> aggregation is implemented; excluded tax policies and external beta/production
> checks remain caller/environment concerns. HTTP, PDF and MCP are future work.
> See the [capability and evidence roadmap](docs/roadmap.md).

## Document flows

| Flow | Public API and endpoint | Guide |
|---|---|---|
| Factura `01` / Boleta `03` | `Client.BuildInvoice`, `SendInvoice`; SOAP billService `sendBill` | Usage below; [individual boletas](docs/receipts.md) |
| Credit/debit notes `07` / `08` | `Client.BuildNote`, `SendNote`; SOAP billService | [Notes](docs/notes.md) |
| Daily summary RC / voided documents RA | `Client.BuildSummary`, `SendSummary`, `BuildVoided`, `SendVoided`; poll `Client.Status` | [Summaries and optional perception](docs/summaries.md) |
| Retention `20` / Perception `40` | `Client.BuildRetention`, `SendRetention`, `BuildPerception`, `SendPerception`; separate otros CPE billService | `transport.OtrosCPEBeta` / `OtrosCPEProduction` |
| Despatch/GRE `09` / `31` | `GREClient.BuildDespatch`, `SendDespatch`, `DespatchStatus`; REST + OAuth2 | `sunat.NewGRE(signer, gre.New(creds))` |
| Document status / CDR recovery | `Client.QueryStatus` / `QueryCDR`; billConsultService, not ticket polling | [Status](docs/status.md) |
| Caller-prepared signed-document ZIP pack | `SOAPClient.SendPack`; poll `SOAPClient.GetStatus` for a raw archive | [Packs](docs/packs.md) |

`Client.Status` parses a single CDR for RC/RA tickets. It is **not** a sendPack
multi-CDR facade: callers must handle the pack's per-document CDRs/report from
`transport.Status.Content` themselves.

## Packages

| Package | Responsibility |
|---|---|
| `sunat` (root) | `Client` document facade and separate `GREClient` |
| `model` | Document/party structs and structural validation; caller-supplied amounts |
| `catalog` | Typed SUNAT catalogs and supported-code validation |
| `calc` | Decimal line calculations (`calc.Line`) and opt-in document aggregation (`calc.Totals`) |
| `ubl` | UBL rendering for supported document types, including SUNAT RC/RA/withholding formats |
| `sign` | Certificate loading (PFX/PEM) and XMLDSig enveloped signature |
| `transport` | SOAP billService, otros CPE and billConsultService with WS-Security |
| `gre` | GRE REST submission/status and OAuth2 token handling |
| `cdr` | CDR (`ApplicationResponse`) parsing; not a pack report aggregator |

Every package can be used on its own. For example, `sign` signs SUNAT XML you
already have. See [calculations and aggregation limits](docs/calculations.md)
for explicit `calc.Totals` / `TotalsResult.Apply` usage. For an unsigned boleta
using `calc.Line` and `calc.Totals`, run `go run ./examples/receipt`: it reads no
certificates/credentials and never sends a network request.

## Usage

Import the root package as `sunat` (`github.com/villano14/sunat-go`). Given a loaded
`sign.Certificate` named `cert`, SOL `transport.Credentials` named `creds`, and a
`model.Invoice` named `inv` with caller-supplied decimal amounts:

```go
client := sunat.New(sign.New(cert), transport.New(transport.BillBeta, creds))
result, err := client.SendInvoice(ctx, inv)
// Store result.XML whenever non-empty, even if err != nil.
if err != nil {
    return err // errors.As still finds *transport.Error.
}
if !result.CDR.Accepted() {
    return fmt.Errorf("invoice rejected: %s: %s", result.CDR.Code, result.CDR.Description)
}
// Store result.CDR.XML as proof of receipt; inspect result.CDR.Notes.
```

Use `client.BuildInvoice(inv)` to render and sign without sending or performing I/O.
The other `Build*` methods similarly do not submit documents.

## Design rules and limits

- Money is `github.com/shopspring/decimal`, never `float64`.
- Every I/O call takes a `context.Context`.
- SUNAT errors keep their numeric code (`*transport.Error`, `cdr.Response.Code`).
- Signers and transports are small interfaces; optional capabilities do not
  expand existing interfaces, and deterministic tests use fakes/local servers.
- `calc.Line` prepares lines; optional `calc.Totals` aggregates rounded line
  amounts and explicit non-base adjustments. Render/build/send never invoke it
  automatically or replace manual amounts. Global tax-base allocation and
  eligibility/rate policies remain caller-owned; see [limits](docs/calculations.md).
- Individual boleta `sendBill` is a supported mechanism, not proof of configured
  endpoint suitability or universal acceptance. Check the CDR, not just transport
  success; RC reporting/annulments and notes retain their distinct purposes.
- Business orchestration, persistence, HTTP serving, PDF and MCP are outside this
  library. A future application API should consume it; MCP follows that API.

## Testing and external proof

```sh
go test ./transport -count=1
go test ./...
```

The default suite is offline when external opt-in flags are unset. Signature
tests independently verify output with `xmlsec1` when installed. The summary
perception XSD test uses `xmllint` and a local schema cache. Tests explicitly
skip when those prerequisites are absent; passing offline tests is not SUNAT
acceptance.

SOAP beta integration tests are opt-in:

```sh
SUNAT_BETA=1 go test -run TestBeta ./...
```

They use public beta credentials (`20000000001MODDATOS` / `moddatos`) and
self-signed test certificates for invoice, summary, retention and perception
flows. No beta consult service is published, and there is no live sendPack
coverage. GRE integration is separately gated by `SUNAT_GRE_BETA=1` and needs
API/SOL credentials; its defaults can target production and issue a real guide.
Do not enable external flags without explicitly choosing the endpoint and
accepting the consequences.

On 2026-11-07 the SOAP beta run accepted invoice, retention and perception
(CDR code `0`), but summary ticket polling failed with HTTP `401`; the run
therefore failed overall. GRE was skipped. Offline race tests and static
analysis passed. See the roadmap for unresolved external proof; this is not
production certification or a completed-library claim.
