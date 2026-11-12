package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
)

// SummaryReference is the document a credit or debit note in a summary
// modifies, e.g. the boleta "B001-1" (type 03).
type SummaryReference struct {
	Type   catalog.DocumentType
	Series string
	Number int64
}

// ID returns the referenced document identifier, e.g. "B001-1".
func (r SummaryReference) ID() string {
	return r.Series + "-" + strconv.FormatInt(r.Number, 10)
}

// SummaryPerception is the optional perception reference in a summary line.
// The caller supplies the rate and amounts; they are rendered with two decimals
// in the summary's currency, without calculating or reconciling totals.
// It is not a standalone Comprobante de Percepción (40).
type SummaryPerception struct {
	Regime         catalog.PerceptionRegime // Catalog 22, e.g. 01
	Rate           decimal.Decimal          // Percentage, e.g. 2 for 2%
	Amount         decimal.Decimal          // Importe de la percepción
	TotalCollected decimal.Decimal          // Total cobrado, incluida la percepción
	TaxableAmount  decimal.Decimal          // Base imponible de la percepción
}

// SummaryLine reports one boleta (03) or note (07, 08) in a Resumen diario.
//
// Amounts are supplied by the caller. Zero amounts are omitted, except that a
// line always reports an operación gravada (code 01) when it reports no other
// operation, because SUNAT requires at least one.
type SummaryLine struct {
	Type   catalog.DocumentType // 03, 07 or 08
	Series string
	Number int64

	ClientIdentityType catalog.IdentityType
	ClientNumber       string

	Reference  *SummaryReference // Required by SUNAT for notes (07, 08)
	Status     catalog.SummaryStatus
	Perception *SummaryPerception // Optional; nil preserves the non-perception XML

	Total            decimal.Decimal // Importe total del comprobante
	TaxedAmount      decimal.Decimal // Operaciones gravadas
	ExoneratedAmount decimal.Decimal // Operaciones exoneradas
	UnaffectedAmount decimal.Decimal // Operaciones inafectas
	ExportAmount     decimal.Decimal // Operaciones de exportación
	FreeAmount       decimal.Decimal // Operaciones gratuitas
	OtherCharges     decimal.Decimal // Otros cargos
	IGV              decimal.Decimal
	ISC              decimal.Decimal
	IVAP             decimal.Decimal // When set, reported instead of IGV
	ICBPER           decimal.Decimal // Impuesto al consumo de bolsas de plástico
	OtherTaxes       decimal.Decimal // Otros tributos
	IGVPercent       decimal.Decimal // e.g. 18; omitted when zero
}

// ID returns the reported document identifier, e.g. "B001-1".
func (l SummaryLine) ID() string {
	return l.Series + "-" + strconv.FormatInt(l.Number, 10)
}

// Summary is a Resumen diario (RC): the boletas and their notes issued on
// GenerationDate, reported to SUNAT on IssueDate.
//
// Dates are rendered as given, so pass them in America/Lima.
type Summary struct {
	Company        Company
	GenerationDate time.Time // Fecha de emisión de los documentos (cbc:ReferenceDate)
	IssueDate      time.Time // Fecha de generación del resumen (cbc:IssueDate)
	Correlative    int       // Daily sequence of the summary, 1 to 99999
	Currency       string    // ISO 4217, defaults to "PEN" when empty
	Lines          []SummaryLine
}

// ID returns the summary identifier, e.g. "RC-20261028-1".
func (s Summary) ID() string {
	return batchID(catalog.BatchSummary, s.IssueDate, s.Correlative)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-RC-20261028-1".
func (s Summary) FileName() string {
	return s.Company.RUC + "-" + s.ID()
}

// VoidedLine is one document a Comunicación de baja voids.
type VoidedLine struct {
	Type   catalog.DocumentType // 01, 07 or 08 (boletas go in a Summary); 20 or 40 in a Reversion
	Series string
	Number int64
	Reason string // Motivo de baja
}

// ID returns the voided document identifier, e.g. "F001-1".
func (l VoidedLine) ID() string {
	return l.Series + "-" + strconv.FormatInt(l.Number, 10)
}

// Voided is a Comunicación de baja (RA).
//
// Dates are rendered as given, so pass them in America/Lima.
type Voided struct {
	Company        Company
	GenerationDate time.Time // Fecha de emisión de los documentos (cbc:ReferenceDate)
	IssueDate      time.Time // Fecha de generación de la comunicación (cbc:IssueDate)
	Correlative    int       // Daily sequence of the communication, 1 to 99999
	Lines          []VoidedLine
}

// ID returns the communication identifier, e.g. "RA-20261028-1".
func (v Voided) ID() string {
	return batchID(catalog.BatchVoided, v.IssueDate, v.Correlative)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-RA-20261028-1".
func (v Voided) FileName() string {
	return v.Company.RUC + "-" + v.ID()
}

// Reversion is a Resumen de reversiones (RR): it reverts Comprobantes de
// Retención (20) and Percepción (40), which a Comunicación de baja cannot void.
// It is sent with sendSummary to the otros CPE billService, like the documents
// it reverts. Lines reuse VoidedLine with Type 20 or 40.
//
// Dates are rendered as given, so pass them in America/Lima.
type Reversion struct {
	Company        Company
	GenerationDate time.Time // Fecha de emisión de los comprobantes (cbc:ReferenceDate)
	IssueDate      time.Time // Fecha de generación de la reversión (cbc:IssueDate)
	Correlative    int       // Daily sequence of the reversion, 1 to 99999
	Lines          []VoidedLine
}

// ID returns the reversion identifier, e.g. "RR-20261028-1".
func (r Reversion) ID() string {
	return batchID(catalog.BatchReversion, r.IssueDate, r.Correlative)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-RR-20261028-1".
func (r Reversion) FileName() string {
	return r.Company.RUC + "-" + r.ID()
}

func batchID(t catalog.BatchType, issue time.Time, correlative int) string {
	return string(t) + "-" + issue.Format("20060102") + "-" + strconv.Itoa(correlative)
}
