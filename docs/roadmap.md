# Roadmap: sunat-go as the single SUNAT protocol source

sunat-go owns SUNAT protocol code (UBL, signing, SOAP/REST, CDR). APISunatGo owns business
logic (tenants, series, persistence, queue, webhooks) and plugs sunat-go in through the ports
of `internal/submission`, behind `SUNAT_ENGINE=internal|sunat-go`.

Each milestone: port the type into sunat-go with golden + xmlsec1 tests and a SUNAT beta test,
then route that type through the APISunatGo adapters and delete the internal code path.

| # | Milestone | Source today in APISunatGo | Notes |
|---|---|---|---|
| 1 | Factura 01 | `internal/ubl`, `internal/xmlsecurity`, `internal/submission` | Done; accepted by SUNAT beta |
| 2 | Boleta 03 hardening + invoice gaps | `internal/ubl/ubl.go` | Allowances/discounts, charges, detraction, ISC/ICBPER, commodity codes (adapter rejects them today) |
| 3 | Notas de crédito/débito 07/08 | `internal/ubl/ubl.go`, `internal/submission/ubl_builder.go` | Reuse invoice lines; add BillingReference and DiscrepancyResponse |
| 4 | Resumen diario (RC) and Comunicación de baja (RA) | `internal/ubl/batches.go`, `internal/summaries` | `sendSummary` + `getStatus` ticket flow in `transport` |
| 5 | Retenciones 20 / Percepciones 40 | `internal/ubl/withholding.go`, `internal/withholdings` | Different billService endpoint |
| 6 | Guía de remisión 09/31 (GRE) | `internal/ubl/batches.go`, `internal/guides`, `submission/gre_rest_client.go` | REST + OAuth2 in a new `gre` package |
| 7 | Consulta CPE / getStatusCdr | `internal/submission` transport methods | Completes `SUNATTransport`; then the decorator can replace the SOAP client |
| 8 | Remove internal engine | all of the above | Drop `SUNAT_ENGINE=internal` once every type runs on sunat-go in production |

Publishing: tag `v0.1.0` and push before APISunatGo can `require` sunat-go; until then local
development uses an untracked `go.work`.
