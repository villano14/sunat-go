package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
)

// PurchaseSeller is the seller of a Liquidación de compra: a natural person
// without RUC, identified by DNI (1) or carné de extranjería (4).
type PurchaseSeller struct {
	IdentityType catalog.IdentityType
	Number       string
	Name         string
	Ubigeo       string               // 6-digit INEI code of the seller's address
	Line         string               // Seller's address
	AddressType  catalog.LocationType // Tipo de domicilio, 01 to 05
}

// PurchaseLocation is the place where the purchase took place.
type PurchaseLocation struct {
	Type   catalog.LocationType // 01 to 05
	Ubigeo string
	Line   string
}

// PurchaseSettlementLine is one detail line of a Liquidación de compra.
//
// IGV applies only to taxed lines (affectation 10); exonerated (20) and
// unaffected (30) lines carry a zero IGV. The income tax retained is reported
// per line under scheme 3000 with the same taxable amount as the IGV.
type PurchaseSettlementLine struct {
	ProductCode      string // Código interno (optional)
	SunatProductCode string // UNSPSC code, catalog 25 (required)
	Unit             string // UN/ECE rec 20, e.g. "KGM"
	Description      string
	Quantity         decimal.Decimal

	UnitValue  decimal.Decimal // Valor unitario (without IGV)
	UnitPrice  decimal.Decimal // Precio unitario: (LineAmount + IGV) / Quantity
	LineAmount decimal.Decimal // Valor de venta: Quantity * UnitValue

	Affectation catalog.IGVAffectation // 10, 20 or 30
	IGVPercent  decimal.Decimal        // e.g. 18; zero unless taxed
	IGV         decimal.Decimal

	IncomeTaxPercent decimal.Decimal // Tasa de retención de renta, e.g. 1.5
	IncomeTax        decimal.Decimal // Renta retenida

	Attributes []ItemAttribute // e.g. gold properties; see GoldAttributes
}

// PurchaseSettlement is a Liquidación de compra (04), UBL SelfBilledInvoice:
// the buyer (Company, with RUC) issues it on behalf of the seller.
//
// Amounts are supplied by the caller (see calc.Settlement). The document's
// TaxTotal reports the IGV; the retained income tax is informative and is
// deducted from the payable amount.
type PurchaseSettlement struct {
	OperationType catalog.OperationType // 0501, 0502 or 0503; defaults to 0501
	Series        string                // "L" followed by 3 alphanumerics, e.g. "L001"
	Number        int64
	IssueDate     time.Time
	Currency      string // ISO 4217, defaults to "PEN" when empty

	Company  Company // Comprador (issuer)
	Seller   PurchaseSeller
	Location PurchaseLocation // Lugar de la operación

	DespatchRefs []DocumentRef // Guías de remisión (09 or 31)
	VehiclePlate string        // Placa when the settlement supports the transport
	Legends      []Legend

	TaxedAmount      decimal.Decimal // Operaciones gravadas
	ExoneratedAmount decimal.Decimal // Operaciones exoneradas
	UnaffectedAmount decimal.Decimal // Operaciones inafectas
	IGV              decimal.Decimal
	IncomeTaxBase    decimal.Decimal // Base de la retención de renta
	IncomeTax        decimal.Decimal // Renta retenida
	LineExtension    decimal.Decimal // Valor de venta
	TaxInclusive     decimal.Decimal // LineExtension + IGV
	Payable          decimal.Decimal // Importe neto: LineExtension - IncomeTax

	Lines []PurchaseSettlementLine
}

// ID returns the document identifier, e.g. "L001-123".
func (p PurchaseSettlement) ID() string {
	return p.Series + "-" + strconv.FormatInt(p.Number, 10)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-04-L001-1".
func (p PurchaseSettlement) FileName() string {
	return p.Company.RUC + "-" + string(catalog.PurchaseSettlement) + "-" + p.ID()
}

// GoldAttributes returns the catalog 55 properties of a gold purchase line,
// omitting empty values: concession code, mineral grade (% of gold), nature of
// the mineral and name of the mining right.
func GoldAttributes(concession, grade, nature, miningRight string) []ItemAttribute {
	var out []ItemAttribute
	for _, p := range []struct{ code, value string }{
		{catalog.PropGoldConcession, concession},
		{catalog.PropGoldGrade, grade},
		{catalog.PropGoldNature, nature},
		{catalog.PropGoldMiningRight, miningRight},
	} {
		if p.value != "" {
			out = append(out, ItemAttribute{Code: p.code, Name: catalog.GoldPropertyName(p.code), Value: p.value})
		}
	}
	return out
}
