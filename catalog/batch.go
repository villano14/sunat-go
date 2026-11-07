package catalog

// SummaryStatus is the item status of a line in a Resumen diario (SUNAT
// catalog 19, "Código de estado del ítem").
type SummaryStatus string

const (
	SummaryAdd    SummaryStatus = "1" // Adicionar: the document is reported for the first time
	SummaryModify SummaryStatus = "2" // Modificar: correct a document reported before
	SummaryVoid   SummaryStatus = "3" // Anulado: the document was voided
)

// Valid reports whether s is one of the catalog 19 codes.
func (s SummaryStatus) Valid() bool {
	switch s {
	case SummaryAdd, SummaryModify, SummaryVoid:
		return true
	}
	return false
}

// BatchType is the prefix of the asynchronous documents SUNAT identifies with
// a ticket, as used in their ID and file name (e.g. "RC-20261027-1").
type BatchType string

const (
	BatchSummary BatchType = "RC" // Resumen diario de boletas
	BatchVoided  BatchType = "RA" // Comunicación de baja
)

// Billing payment instruction codes of a Resumen diario line (catalog 11).
const (
	PaymentTaxed      = "01" // Operaciones gravadas
	PaymentExonerated = "02" // Operaciones exoneradas
	PaymentUnaffected = "03" // Operaciones inafectas
	PaymentExport     = "04" // Operaciones de exportación
	PaymentFree       = "05" // Operaciones gratuitas
)

// Additional tax schemes reported by a Resumen diario line.
var (
	SchemeIVAP   = TaxScheme{ID: "1016", Name: "IVAP", Code: "VAT"}
	SchemeISC    = TaxScheme{ID: "2000", Name: "ISC", Code: "EXC"}
	SchemeOthers = TaxScheme{ID: "9999", Name: "OTROS", Code: "OTH"}
	SchemeICBPER = TaxScheme{ID: "7152", Name: "ICBPER", Code: "OTH"}
)
