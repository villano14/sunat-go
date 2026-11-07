package catalog

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Documents a Comprobante de Retención (20) or Percepción (40) may reference.
// They extend DocumentType (catalog 01), which already holds Invoice (01),
// Receipt (03), CreditNote (07) and DebitNote (08).
const (
	CashRegisterTicket DocumentType = "12" // Ticket de máquina registradora
	UtilityReceipt     DocumentType = "14" // Recibo de servicios públicos
)

// Withholding document types, the "tipo de comprobante" part of the file name
// RUC-TT-SSSS-NNNN.
const (
	Retention  DocumentType = "20" // Comprobante de Retención
	Perception DocumentType = "40" // Comprobante de Percepción
)

// RetentionRegime is SUNAT catalog 23, the retention regime and its rate.
type RetentionRegime string

const (
	RetentionRate3 RetentionRegime = "01" // Tasa 3%
	RetentionRate6 RetentionRegime = "02" // Tasa 6%
)

// PerceptionRegime is SUNAT catalog 22, the perception regime and its rate.
type PerceptionRegime string

const (
	PerceptionInternalSale PerceptionRegime = "01" // Percepción venta interna, tasa 2%
	PerceptionFuel         PerceptionRegime = "02" // Percepción a la adquisición de combustible, tasa 1%
	PerceptionSpecialRate  PerceptionRegime = "03" // Percepción realizada al agente de percepción con tasa especial, tasa 0.5%
)

// RetentionRate returns the percentage (e.g. 3 for 3%) of a retention regime.
func RetentionRate(r RetentionRegime) (decimal.Decimal, error) {
	switch r {
	case RetentionRate3:
		return decimal.NewFromInt(3), nil
	case RetentionRate6:
		return decimal.NewFromInt(6), nil
	}
	return decimal.Zero, fmt.Errorf("catalog: unsupported retention regime %q", string(r))
}

// PerceptionRate returns the percentage (e.g. 2 for 2%) of a perception regime.
func PerceptionRate(r PerceptionRegime) (decimal.Decimal, error) {
	switch r {
	case PerceptionInternalSale:
		return decimal.NewFromInt(2), nil
	case PerceptionFuel:
		return decimal.NewFromInt(1), nil
	case PerceptionSpecialRate:
		return decimal.RequireFromString("0.5"), nil
	}
	return decimal.Zero, fmt.Errorf("catalog: unsupported perception regime %q", string(r))
}
