package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
)

// Payment is one payment (retention) or collection (perception) made against
// a related document. Currency is the ISO 4217 code of the related document.
type Payment struct {
	Currency string
	Amount   decimal.Decimal
	Date     time.Time
}

// ExchangeRate is the exchange rate applied when the related document is not
// in soles, e.g. USD to PEN. Rate is rendered with six decimals.
type ExchangeRate struct {
	SourceCurrency string // Moneda de referencia, e.g. "USD"
	TargetCurrency string // Moneda objetivo, e.g. "PEN"
	Rate           decimal.Decimal
	Date           time.Time
}

// RetentionDetail is one document a retention applies to.
//
// The retention block (RetentionDate, Retained, NetPayable, Exchange) is only
// rendered when RetentionDate is set and Retained and NetPayable are non-zero,
// as in Greenter.
type RetentionDetail struct {
	DocumentType catalog.DocumentType // Related document, e.g. catalog.Invoice
	Number       string               // Series-number, e.g. "F001-1"
	IssueDate    time.Time
	Total        decimal.Decimal // Importe total del documento relacionado
	Currency     string          // ISO 4217 of the related document
	Payments     []Payment       // Pago: at most one per detail (SUNAT schema); repeat the document for each payment

	RetentionDate time.Time
	Retained      decimal.Decimal // Importe retenido, in PEN
	NetPayable    decimal.Decimal // Importe pagado neto de la retención, in PEN
	Exchange      *ExchangeRate   // Required when Currency is not PEN
}

// Retention is a Comprobante de Retención (20), issued by the retention agent
// (Company) to its supplier. The caller supplies every amount.
type Retention struct {
	Series    string // e.g. "R001"
	Number    int64
	IssueDate time.Time

	Company  Company
	Supplier Client // Proveedor retenido

	Regime        catalog.RetentionRegime // Catalog 23
	Rate          decimal.Decimal         // Percentage, e.g. 3; defaults to the regime's rate when zero
	Note          string                  // Observación
	TotalRetained decimal.Decimal         // Importe total retenido, in PEN
	TotalPaid     decimal.Decimal         // Importe total pagado, in PEN

	Details []RetentionDetail
}

// ID returns the document identifier, e.g. "R001-123".
func (r Retention) ID() string {
	return r.Series + "-" + strconv.FormatInt(r.Number, 10)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-20-R001-1".
func (r Retention) FileName() string {
	return r.Company.RUC + "-" + string(catalog.Retention) + "-" + r.ID()
}

// PerceptionDetail is one document a perception applies to.
//
// The perception block (PerceptionDate, Perceived, NetCollectable, Exchange)
// is only rendered when PerceptionDate is set and Perceived and NetCollectable
// are non-zero, as in Greenter.
type PerceptionDetail struct {
	DocumentType catalog.DocumentType // Related document, e.g. catalog.Invoice
	Number       string               // Series-number, e.g. "F001-1"
	IssueDate    time.Time
	Total        decimal.Decimal // Importe total del documento relacionado
	Currency     string          // ISO 4217 of the related document
	Collections  []Payment       // Cobros

	PerceptionDate time.Time
	Perceived      decimal.Decimal // Importe percibido, in PEN
	NetCollectable decimal.Decimal // Importe a cobrar neto de la percepción, in PEN
	Exchange       *ExchangeRate   // Required when Currency is not PEN
}

// Perception is a Comprobante de Percepción (40), issued by the perception
// agent (Company) to its customer. The caller supplies every amount.
type Perception struct {
	Series    string // e.g. "P001"
	Number    int64
	IssueDate time.Time

	Company  Company
	Customer Client // Cliente percibido

	Regime         catalog.PerceptionRegime // Catalog 22
	Rate           decimal.Decimal          // Percentage, e.g. 2; defaults to the regime's rate when zero
	Note           string                   // Observación
	TotalPerceived decimal.Decimal          // Importe total percibido, in PEN
	TotalCollected decimal.Decimal          // Importe total cobrado, in PEN

	Details []PerceptionDetail
}

// ID returns the document identifier, e.g. "P001-123".
func (p Perception) ID() string {
	return p.Series + "-" + strconv.FormatInt(p.Number, 10)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-40-P001-1".
func (p Perception) FileName() string {
	return p.Company.RUC + "-" + string(catalog.Perception) + "-" + p.ID()
}
