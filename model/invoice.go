// Package model defines the electronic documents and parties sent to SUNAT.
//
// Amounts are supplied by the caller; this package does not compute totals.
package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
)

// Address is a fiscal address. Ubigeo is the 6-digit INEI code.
type Address struct {
	Ubigeo            string
	Department        string
	Province          string
	District          string
	Urbanization      string
	Line              string
	CountryCode       string // ISO 3166-1 alpha-2, defaults to "PE" when empty
	EstablishmentCode string // Código de establecimiento anexo, defaults to "0000"
}

// Company is the issuer.
type Company struct {
	RUC       string
	LegalName string // Razón social
	TradeName string // Nombre comercial
	Address   Address
}

// Client is the customer.
type Client struct {
	IdentityType catalog.IdentityType
	Number       string
	Name         string
	Address      *Address
}

// Legend is a document note (cbc:Note) such as the amount in words.
type Legend struct {
	Code  catalog.LegendCode
	Value string
}

// Installment is one credit payment quota.
type Installment struct {
	Amount  decimal.Decimal
	DueDate time.Time
}

// PaymentTerms describes the payment method. Credit is true for "Credito",
// in which case Amount is the pending total and Installments its quotas.
type PaymentTerms struct {
	Credit       bool
	Amount       decimal.Decimal
	Installments []Installment
}

// InvoiceLine is one detail line.
type InvoiceLine struct {
	ProductCode string
	Unit        string // UN/ECE rec 20, e.g. "NIU", "ZZ"
	Description string
	Quantity    decimal.Decimal

	UnitValue decimal.Decimal // Valor unitario (without IGV)
	UnitPrice decimal.Decimal // Precio unitario (with IGV)
	PriceType catalog.PriceType

	Affectation catalog.IGVAffectation
	TaxBase     decimal.Decimal // Base imponible IGV
	IGVPercent  decimal.Decimal // e.g. 18
	IGV         decimal.Decimal
	TotalTaxes  decimal.Decimal
	LineAmount  decimal.Decimal // Valor de venta (quantity * unit value)
}

// Invoice is a Factura (01) or Boleta (03).
type Invoice struct {
	Type          catalog.DocumentType
	OperationType catalog.OperationType
	Series        string
	Number        int64
	IssueDate     time.Time
	DueDate       *time.Time
	Currency      string // ISO 4217, e.g. "PEN"

	Company      Company
	Client       Client
	PaymentTerms *PaymentTerms

	TaxedAmount      decimal.Decimal // Operaciones gravadas
	ExoneratedAmount decimal.Decimal // Operaciones exoneradas
	UnaffectedAmount decimal.Decimal // Operaciones inafectas
	ExportAmount     decimal.Decimal // Operaciones de exportación
	FreeAmount       decimal.Decimal // Operaciones gratuitas
	IGV              decimal.Decimal
	TotalTaxes       decimal.Decimal
	LineExtension    decimal.Decimal // Valor de venta
	TaxInclusive     decimal.Decimal // Subtotal (valor de venta + impuestos)
	Payable          decimal.Decimal // Importe total

	Lines   []InvoiceLine
	Legends []Legend
}

// ID returns the document identifier, e.g. "F001-123".
func (i Invoice) ID() string {
	return i.Series + "-" + strconv.FormatInt(i.Number, 10)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-01-F001-1".
func (i Invoice) FileName() string {
	return i.Company.RUC + "-" + string(i.Type) + "-" + i.ID()
}
