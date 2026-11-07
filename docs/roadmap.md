# Roadmap: library capability before service integration

sunat-go owns SUNAT protocol code (UBL, signing, SOAP/REST and CDR). Supported
library flows are implemented and exercised offline, but **library completion,
live acceptance and APISunatGo integration are separate claims**. Opt-in document
aggregation is implemented with explicit policy limits; HTTP, PDF and MCP remain
outside this module and come after the library work.

## Implemented library capabilities and evidence

| # | Milestone / API | Offline evidence | External proof / remaining limits |
|---|---|---|---|
| 1 | Factura `01`: `BuildInvoice` / `SendInvoice` | XML values/golden, signing, fake pipeline and SOAP tests | Revalidated on 2026-11-07: `TestBetaSendInvoice` accepted with CDR code `0`; beta only, not production proof |
| 2 | Individual boleta `03`: `ubl.Invoice`, `BuildInvoice` / `SendInvoice` | Explicit `B001` / DNI, namespaces, manual amounts, signing/tamper, SendBill filename/context and synthetic accepted/observed/rejected CDR/error tests | Individual `sendBill` is supported; configured endpoint suitability and actual boleta acceptance are not established by code or the prior factura beta run. RC reporting/annulments and notes remain distinct; see [boletas](receipts.md) |
| 2 | Invoice fields and optional `calc.Line` / `calc.Totals` | Decimal rounding, mixed categories/rates, free/IVAP separation, explicit adjustments, pure application and manual-amount compatibility tests | Aggregation is opt-in, never automatic in render/build/send. Field presence and arithmetic are not tax-policy/eligibility or live acceptance proof; see [calculations](calculations.md) |
| 3 | Credit/debit notes `07` / `08`: `BuildNote` / `SendNote` | Both UBL roots, references/reasons, signing/tamper and pipeline failure tests | No note beta/production acceptance claimed; see [notes](notes.md) |
| 4 | RC `model.Summary`, `ubl.Summary`, `Client.SendSummary`; RA `model.Voided`, `ubl.Voided`, `Client.SendVoided` | Golden/value/order and signing tests; fake-transport ticket/CDR tests | 2026-11-07 beta run obtained an RC ticket, but polling failed with HTTP `401`; end-to-end RC acceptance is unverified. No RA beta test yet |
| 4 | `BatchTransport.SendSummary` / `GetStatus`; `Client.Status` | Local HTTP success/fault/error tests; fake facade tests for `98`, processed and rejected states | `Client.Status` parses one CDR for RC/RA; processed `0` alone is not document acceptance |
| 4 | Optional summary perception reference | XML, nil-preservation and cached-XSD tests | Caller supplies amounts/rate; optional tooling/cache required for XSD check. No live acceptance claim; see [summaries](summaries.md) |
| 4 | Optional `PackTransport.SendPack` / `SOAPClient.GetStatus` | Exact ZIP bytes/name, namespace/auth/action, ticket, faults, HTTP/context and raw multi-CDR archive tests | Transport-only; caller builds signed-document ZIP and handles per-document CDRs/report. No live sendPack or attachment compatibility proof; see [packs](packs.md) |
| 5 | Retention `20` / Perception `40`: build/send methods | XML golden/value/order, signing, pipeline and SOAP tests | Separate otros CPE endpoint; 2026-11-07 beta retention and perception tests accepted with CDR code `0`; not production proof |
| 6 | GRE `09` / `31`: `GREClient`, `gre` REST/OAuth2 | XML golden/value, signing, pipeline and local REST/token tests | GRE integration is separately opt-in and credential-dependent; defaults may issue a real guide in production. No external check performed here |
| 7 | `ConsultTransport.GetStatusCdr` / `Client.QueryCDR` | Local HTTP and fake-facade tests, including optional CDR | Production consult service only; no published beta consult endpoint, no real-SOL production verification |
| 7 | `CPEStatusTransport.GetStatusCpe` / `Client.QueryStatus` | Status-only codes/messages, errors, escaping/auth and endpoint-isolation tests | Not ticket polling or CDR recovery; no live availability/authorization claim; see [status](status.md) |
| 8 | Remove APISunatGo internal engine | Not implemented or verified by this repository's tests | External integration/replacement remains future work |

Signing tests use `xmlsec1` when installed and skip otherwise. The summary
perception schema test uses `xmllint --nonet` and the local ignored schema cache;
it skips if prerequisites are absent. These checks are independent of SUNAT's
live business validation. Ordinary `go test ./...` does not establish that any
opt-in external test ran.

## What remains before declaring the library complete

- **Calculation policy:** `calc.Totals` now sums rounded line amounts/taxes,
  handles explicit non-base allowances/charges, gross prepayments and rounding,
  and exposes `TotalsResult.Apply` for opt-in invoice assembly. Global
  tax-base-changing discount/charge or advance allocations, perception/detraction
  eligibility/calculation, conversion and other legal applicability decisions
  remain explicit caller policy. Manual amounts are still supported. See
  [calculations](calculations.md) for exact supported arithmetic and exclusions.
- **External checks:** the 2026-11-07 SOAP beta run exited `1`: invoice,
  retention and perception were accepted, but RC ticket polling returned HTTP
  `401`. Its cause is not established; the beta WSDL declares `getStatus`,
  while SUNAT's published beta guidance does not promise summary coverage.
  No authentication change or retry is justified by this result alone.
  GRE was explicitly skipped without its opt-in credentials. GRE and
  real-credential production consult checks still require an appropriate
  authorized environment. Beta success is not production acceptance, and no
  production submission is authorized by this plan.
- **sendPack application policy:** eligibility, limits, ZIP assembly, scheduling
  and full response-archive handling belong to the caller. No batch signing
  policy or multi-CDR facade was added. Transport support does not establish
  endpoint/attachment interoperability or taxpayer authorization.
- **Validation scope:** existing checks and offline fixtures do not prove every
  SUNAT business rule or all combinations of optional fields. Further gaps must
  be demonstrated and scoped, not hidden behind a blanket completion claim.

## Final verification gate (2026-11-07)

The complete implementation working tree passed `go test -race ./...`,
`go vet ./...` and `git diff --check` after the totals and individual-receipt
work. XML signature and summary-perception schema checks also executed with
the required local tools available. These are observed local checks, not
production certification.

The required final independent security/concurrency verification did **not**
execute: preparation of its isolated workspace failed on a runtime Unix socket.
No independent final PASS is claimed, and its unavailable attempt was not
retried or replaced with parent self-checks. Automated native review assessment
was also unavailable; no native review approval was issued. Local work-unit
commits preserve implementation and attribution, not release authorization.
The earlier RC beta HTTP `401` and credential-dependent checks remain subject
to the external-proof limits above.

## APISunatGo migration plan — external, not completed here

The intended boundary remains: APISunatGo owns tenants, series, persistence,
queues and webhooks. The prior plan routes sunat-go through `internal/submission`
ports behind `SUNAT_ENGINE=internal|sunat-go`, then deletes each internal path
only after its replacement is verified. APISunatGo was not accessed in this
work; the historical source map below is planning context, not a current audit.

| Milestone | Prior APISunatGo source map |
|---|---|
| 1 | `internal/ubl`, `internal/xmlsecurity`, `internal/submission` |
| 2 | `internal/ubl/ubl.go`; prior adapter restrictions require external recheck |
| 3 | `internal/ubl/ubl.go`, `internal/submission/ubl_builder.go` |
| 4 | `internal/ubl/batches.go`, `internal/summaries` |
| 5 | `internal/ubl/withholding.go`, `internal/withholdings` |
| 6 | `internal/ubl/batches.go`, `internal/guides`, `submission/gre_rest_client.go` |
| 7 | `internal/submission` transport methods and `SUNATTransport` decorator |
| 8 | All preceding paths; remove `SUNAT_ENGINE=internal` only after production replacement |

The old milestone-7 gate combined a real-SOL `getStatusCdr` production check
with wiring the APISunatGo decorator. Both remain external work, not evidence
that the library APIs are absent. New status-only consult and pack operations
close specific protocol gaps without completing that integration gate.

The original publishing target was `v0.1.0`, with an untracked `go.work` for local
cross-repository development before release. No tag, publish, push, adapter
migration or internal-engine removal is performed or claimed here. Post-library
HTTP APIs, PDF generation and MCP tooling require their own scope and
verification outside this module. The recommended future application API should
consume sunat-go rather than duplicate its protocol/calculation code; MCP should
follow that API.
