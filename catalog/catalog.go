// Package catalog defines typed SUNAT catalogs used by electronic documents.
//
// Catalog numbers follow the SUNAT UBL 2.1 annexes ("Catálogo N°").
package catalog

import "fmt"

// DocumentType is SUNAT catalog 01.
type DocumentType string

const (
	Invoice    DocumentType = "01" // Factura
	Receipt    DocumentType = "03" // Boleta de venta
	CreditNote DocumentType = "07" // Nota de crédito
	DebitNote  DocumentType = "08" // Nota de débito
)

// IdentityType is SUNAT catalog 06.
type IdentityType string

const (
	IdentityNone IdentityType = "0" // Sin documento (non-domiciled)
	DNI          IdentityType = "1"
	ForeignCard  IdentityType = "4" // Carné de extranjería
	RUC          IdentityType = "6"
	Passport     IdentityType = "7"
)

// IGVAffectation is SUNAT catalog 07.
type IGVAffectation string

const (
	TaxedOnerous      IGVAffectation = "10" // Gravado - Operación onerosa
	TaxedFree         IGVAffectation = "11" // Gravado - Retiro por premio
	ExoneratedOnerous IGVAffectation = "20" // Exonerado - Operación onerosa
	UnaffectedOnerous IGVAffectation = "30" // Inafecto - Operación onerosa
	UnaffectedFree    IGVAffectation = "31" // Inafecto - Retiro por bonificación
	Export            IGVAffectation = "40" // Exportación
)

// OperationType is SUNAT catalog 51.
type OperationType string

const (
	InternalSale OperationType = "0101" // Venta interna
)

// LegendCode is SUNAT catalog 52.
type LegendCode string

const (
	AmountInWords LegendCode = "1000" // Monto en letras
)

// PriceType is SUNAT catalog 16.
type PriceType string

const (
	UnitPriceWithTax PriceType = "01" // Precio unitario (incluye IGV)
	ReferenceValue   PriceType = "02" // Valor referencial en operaciones no onerosas
)

// TaxScheme is a SUNAT catalog 05 entry.
type TaxScheme struct {
	ID   string // e.g. "1000"
	Name string // e.g. "IGV"
	Code string // UN/ECE 5153 code, e.g. "VAT"
}

var (
	SchemeIGV        = TaxScheme{ID: "1000", Name: "IGV", Code: "VAT"}
	SchemeExport     = TaxScheme{ID: "9995", Name: "EXP", Code: "FRE"}
	SchemeFree       = TaxScheme{ID: "9996", Name: "GRA", Code: "FRE"}
	SchemeExonerated = TaxScheme{ID: "9997", Name: "EXO", Code: "VAT"}
	SchemeUnaffected = TaxScheme{ID: "9998", Name: "INA", Code: "FRE"}
)

// SchemeFor returns the tax scheme a line with the given IGV affectation reports.
func SchemeFor(a IGVAffectation) (TaxScheme, error) {
	switch a {
	case TaxedOnerous:
		return SchemeIGV, nil
	case TaxedFree, UnaffectedFree:
		return SchemeFree, nil
	case ExoneratedOnerous:
		return SchemeExonerated, nil
	case UnaffectedOnerous:
		return SchemeUnaffected, nil
	case Export:
		return SchemeExport, nil
	}
	return TaxScheme{}, fmt.Errorf("catalog: unsupported IGV affectation %q", string(a))
}
