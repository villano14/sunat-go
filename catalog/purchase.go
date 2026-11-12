package catalog

// PurchaseSettlement is the Liquidación de compra (catalog 01, code 04): the
// buyer issues it for purchases from natural persons without RUC.
const PurchaseSettlement DocumentType = "04"

// Operation types of a Liquidación de compra (catalog 51).
const (
	PurchaseInternal OperationType = "0501" // Compra interna
	PurchaseAdvance  OperationType = "0502" // Anticipos
	PurchaseGold     OperationType = "0503" // Compra de oro
)

// SchemeIncomeTax is the income tax retained in a Liquidación de compra
// (catalog 05, code 3000). It is reported in the TaxTotal but is not added to
// its TaxAmount, which is the IGV only.
var SchemeIncomeTax = TaxScheme{ID: "3000", Name: "IR", Code: "TOX"}

// LocationType is the "tipo de lugar / domicilio" code of a Liquidación de
// compra: the seller's address type and the place of the operation. SUNAT
// accepts the codes 01 to 05.
type LocationType string

// Valid reports whether t is one of the codes 01 to 05.
func (t LocationType) Valid() bool {
	switch t {
	case "01", "02", "03", "04", "05":
		return true
	}
	return false
}

// Item properties (catalog 55) of a gold purchase (operation type 0503). The
// names are the ones of the Liquidación de compra accepted by SUNAT.
const (
	PropGoldConcession  = "6000" // Código único de concesión minera
	PropGoldGrade       = "6004" // Ley mineral (% de concentración de oro)
	PropGoldNature      = "6005" // Naturaleza del mineral
	PropGoldMiningRight = "6006" // Nombre del derecho minero
)

// GoldPropertyName returns the catalog 55 name of a gold property code, or ""
// when code is not one of the Prop* gold constants.
func GoldPropertyName(code string) string {
	switch code {
	case PropGoldConcession:
		return "Comercialización de Oro :  Código Unico Concesión Minera"
	case PropGoldGrade:
		return "Comercialización de Oro : Ley Mineral (% concent. oro)"
	case PropGoldNature:
		return "Comercialización de Oro : Naturaleza del mineral"
	case PropGoldMiningRight:
		return "Comercialización de Oro : Nombre del derecho minero"
	}
	return ""
}
