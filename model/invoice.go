// Package model defines the electronic documents and parties sent to SUNAT.
//
// Amounts are supplied by the caller; this package does not compute totals
// (see package calc for helpers that do).
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
	Email     string
	Phone     string
}

// Client is the customer.
type Client struct {
	IdentityType catalog.IdentityType
	Number       string
	Name         string
	Address      *Address
	Email        string
	Phone        string
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

// AllowanceCharge is a discount or a charge (cac:AllowanceCharge), at line or
// document level. Factor and BaseAmount are optional: Factor is rendered when
// non-zero and BaseAmount always (SUNAT requires it for document-level ones).
type AllowanceCharge struct {
	Charge     bool // true for a charge (cargo), false for a discount (descuento)
	Reason     catalog.AllowanceChargeReason
	Factor     decimal.Decimal // e.g. 0.10 for 10 %
	Amount     decimal.Decimal
	BaseAmount decimal.Decimal
}

// DocumentRef points to a related document (guía de remisión, otros).
type DocumentRef struct {
	Type   string // catalog 12 code, e.g. "09" for a guía de remisión
	Number string // e.g. "T001-45"
}

// Prepayment is an advance (anticipo) already invoiced and deducted here.
type Prepayment struct {
	DocType string          // "02" factura or "03" boleta (catalog 12)
	Number  string          // e.g. "F001-12"
	Total   decimal.Decimal // amount deducted, with IGV
}

// Detraction is the SPOT information (operation type 1001-1004).
type Detraction struct {
	MeanOfPayment string          // catalog 59, e.g. "001"
	BankAccount   string          // Banco de la Nación account of the supplier
	GoodCode      string          // catalog 54, e.g. "037"
	Percent       decimal.Decimal // e.g. 12
	Amount        decimal.Decimal // in PEN
}

// InvoicePerception is the perception included in an invoice (operation type 2001).
type InvoicePerception struct {
	Regime  catalog.AllowanceChargeReason // 51, 52 or 53
	Percent decimal.Decimal               // e.g. 2 (rendered as factor 0.02)
	Amount  decimal.Decimal               // perception amount, PEN
	Base    decimal.Decimal               // base amount, PEN
	Total   decimal.Decimal               // base + perception, PEN
}

// ItemAttribute is an additional item property (cac:AdditionalItemProperty).
type ItemAttribute struct {
	Code      string // NameCode, SUNAT catalog 55
	Name      string
	Value     string
	StartDate *time.Time
	EndDate   *time.Time
	Duration  int // days
}

// InvoiceLine is one detail line. The same structure serves the lines of
// notas de crédito and débito.
type InvoiceLine struct {
	ProductCode string
	Unit        string // UN/ECE rec 20, e.g. "NIU", "ZZ"
	Description string
	Quantity    decimal.Decimal

	UnitValue decimal.Decimal // Valor unitario (without IGV)
	UnitPrice decimal.Decimal // Precio unitario (with IGV); for free operations, the reference value
	PriceType catalog.PriceType

	Affectation catalog.IGVAffectation
	TaxBase     decimal.Decimal // Base imponible IGV
	IGVPercent  decimal.Decimal // e.g. 18
	IGV         decimal.Decimal
	TotalTaxes  decimal.Decimal
	LineAmount  decimal.Decimal // Valor de venta (quantity * unit value, less line discounts)

	// Discounts and charges applied to this line (catalog 53).
	AllowanceCharges []AllowanceCharge

	// ISC (Impuesto Selectivo al Consumo). Rendered when ISC is non-zero.
	ISC        decimal.Decimal
	ISCBase    decimal.Decimal
	ISCPercent decimal.Decimal
	ISCSystem  catalog.ISCSystem

	// Other taxes (scheme 9999). Rendered when OtherTax is non-zero.
	OtherTax        decimal.Decimal
	OtherTaxBase    decimal.Decimal
	OtherTaxPercent decimal.Decimal

	// ICBPER (impuesto al consumo de bolsas de plástico). Rendered when ICBPER
	// is non-zero; ICBPERFactor is the amount per bag.
	ICBPER       decimal.Decimal
	ICBPERFactor decimal.Decimal

	GS1Code          string          // cac:StandardItemIdentification
	SunatProductCode string          // UNSPSC code, catalog 25 (cac:CommodityClassification)
	Attributes       []ItemAttribute // cac:AdditionalItemProperty
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
	Seller       *Client  // cac:SellerSupplierParty (optional)
	Delivery     *Address // delivery address (optional)
	PaymentTerms *PaymentTerms

	TaxedAmount      decimal.Decimal // Operaciones gravadas
	ExoneratedAmount decimal.Decimal // Operaciones exoneradas
	UnaffectedAmount decimal.Decimal // Operaciones inafectas
	ExportAmount     decimal.Decimal // Operaciones de exportación
	FreeAmount       decimal.Decimal // Operaciones gratuitas
	FreeIGV          decimal.Decimal // IGV of the free operations (GRA)
	IGV              decimal.Decimal
	ISC              decimal.Decimal // Total ISC (rendered when non-zero)
	ISCBase          decimal.Decimal
	IVAP             decimal.Decimal // Total IVAP (rendered when non-zero)
	IVAPBase         decimal.Decimal
	OtherTaxes       decimal.Decimal // Total of other taxes, scheme 9999
	OtherTaxesBase   decimal.Decimal
	ICBPER           decimal.Decimal // Total ICBPER
	TotalTaxes       decimal.Decimal
	LineExtension    decimal.Decimal // Valor de venta
	TaxInclusive     decimal.Decimal // Subtotal (valor de venta + impuestos)
	AllowanceTotal   decimal.Decimal // Total of discounts that do not reduce the taxable base
	ChargeTotal      decimal.Decimal // Total of charges
	PrepaidTotal     decimal.Decimal // Total of prepayments deducted
	Rounding         decimal.Decimal // PayableRoundingAmount
	Payable          decimal.Decimal // Importe total

	// Document-level discounts and charges (catalog 53), including the IGV
	// retention (reason 62).
	AllowanceCharges []AllowanceCharge
	Prepayments      []Prepayment
	Detraction       *Detraction
	Perception       *InvoicePerception

	Observation    string        // free note (cbc:Note without language)
	OrderReference string        // purchase order number
	DespatchRefs   []DocumentRef // guías de remisión
	RelatedDocs    []DocumentRef // other related documents

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
