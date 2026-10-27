# sunat-go

Go library for Peruvian electronic invoicing (SUNAT): build UBL 2.1 XML, sign it with
XMLDSig, send it to SUNAT web services, and read the CDR response.

Inspired by [Greenter](https://github.com/thegreenter/greenter) (PHP), designed as an
idiomatic Go library: small packages, no framework, no database.

> Status: milestone 1 in progress — Factura (01) with IGV.

## Packages

| Package | Responsibility |
|---|---|
| `sunat` (root) | `Client` facade: build → sign → send → parse CDR |
| `model` | Document structs (invoice, company, client, lines) |
| `catalog` | Typed SUNAT catalogs (document types, IGV affectation, units) |
| `ubl` | UBL 2.1 XML generation |
| `sign` | Certificate loading (PFX/PEM) and XMLDSig enveloped signature |
| `transport` | SOAP `billService` client with WS-Security |
| `cdr` | CDR (`ApplicationResponse`) parsing |

Every package can be used on its own. For example, `sign` signs any SUNAT XML you
already have.

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

## Design rules

- Money is `github.com/shopspring/decimal`, never `float64`.
- Every I/O call takes a `context.Context`.
- SUNAT errors keep their numeric code (`*transport.Error`, `cdr.Response.Code`).
- `Signer` and `Transport` are small interfaces, so tests never hit SUNAT.
- The library does not compute totals: the caller supplies amounts, as in Greenter.

## Testing

```sh
go test ./...
```

Signature tests verify output with `xmlsec1` when it is installed.

The SUNAT beta integration test is opt-in:

```sh
SUNAT_BETA=1 go test -run TestBeta ./...
```

It uses the public beta credentials (`20000000001MODDATOS` / `moddatos`) and a
self-signed test certificate.
