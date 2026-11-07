package catalog

// AllowanceChargeReason is SUNAT catalog 53, the reason code of a discount or
// charge (cac:AllowanceCharge) at line or document level.
type AllowanceChargeReason string

const (
	DiscountItemTaxBase      AllowanceChargeReason = "00" // Descuento por ítem que afecta la base imponible del IGV/IVAP
	DiscountItem             AllowanceChargeReason = "01" // Descuento por ítem que no afecta la base imponible
	DiscountGlobalTaxBase    AllowanceChargeReason = "02" // Descuento global que afecta la base imponible
	DiscountGlobal           AllowanceChargeReason = "03" // Descuento global que no afecta la base imponible
	DiscountAdvanceTaxed     AllowanceChargeReason = "04" // Descuento global por anticipos gravados
	DiscountAdvanceExonerate AllowanceChargeReason = "05" // Descuento global por anticipos exonerados
	DiscountAdvanceUnaffect  AllowanceChargeReason = "06" // Descuento global por anticipos inafectos
	ChargeFISE               AllowanceChargeReason = "45" // Fondo de Inversión Social de Electrificación
	ChargeTip                AllowanceChargeReason = "46" // Recargo al consumo y/o propinas
	ChargeItemTaxBase        AllowanceChargeReason = "47" // Cargo por ítem que afecta la base imponible
	ChargeItem               AllowanceChargeReason = "48" // Cargo por ítem que no afecta la base imponible
	ChargeGlobalTaxBase      AllowanceChargeReason = "49" // Cargo global que afecta la base imponible
	ChargeGlobal             AllowanceChargeReason = "50" // Cargo global que no afecta la base imponible
	ChargePerceptionSale     AllowanceChargeReason = "51" // Percepción venta interna
	ChargePerceptionFuel     AllowanceChargeReason = "52" // Percepción a la adquisición de combustible
	ChargePerceptionSpecial  AllowanceChargeReason = "53" // Percepción realizada al agente de percepción con tasa especial
	RetentionIGV             AllowanceChargeReason = "62" // Retención del IGV (descuento global)
)

// ISCSystem is SUNAT catalog 08, the ISC calculation system of a line.
type ISCSystem string

const (
	ISCFixedAmount ISCSystem = "01" // Sistema al valor (Apéndice IV, lit. A - T.U.O. IGV e ISC)
	ISCMinimum     ISCSystem = "02" // Aplicación del Monto Fijo (Apéndice IV, lit. B)
	ISCRetail      ISCSystem = "03" // Sistema de Precios de Venta al Público (Apéndice IV, lit. C)
)

// CreditNoteReason is SUNAT catalog 09, the reason of a Nota de crédito.
type CreditNoteReason string

const (
	CreditCancel         CreditNoteReason = "01" // Anulación de la operación
	CreditCancelWrongRUC CreditNoteReason = "02" // Anulación por error en el RUC
	CreditFixDescription CreditNoteReason = "03" // Corrección por error en la descripción
	CreditGlobalDiscount CreditNoteReason = "04" // Descuento global
	CreditItemDiscount   CreditNoteReason = "05" // Descuento por ítem
	CreditFullReturn     CreditNoteReason = "06" // Devolución total
	CreditItemReturn     CreditNoteReason = "07" // Devolución por ítem
	CreditBonus          CreditNoteReason = "08" // Bonificación
	CreditValueDecrease  CreditNoteReason = "09" // Disminución en el valor
	CreditOther          CreditNoteReason = "10" // Otros conceptos
	CreditExportAdjust   CreditNoteReason = "11" // Ajustes de operaciones de exportación
	CreditIVAPAdjust     CreditNoteReason = "12" // Ajustes afectos al IVAP
	CreditPaymentFix     CreditNoteReason = "13" // Corrección del monto neto pendiente de pago y/o fechas de vencimiento
)

// Valid reports whether r is a catalog 09 code.
func (r CreditNoteReason) Valid() bool {
	switch r {
	case CreditCancel, CreditCancelWrongRUC, CreditFixDescription, CreditGlobalDiscount,
		CreditItemDiscount, CreditFullReturn, CreditItemReturn, CreditBonus, CreditValueDecrease,
		CreditOther, CreditExportAdjust, CreditIVAPAdjust, CreditPaymentFix:
		return true
	}
	return false
}

// DebitNoteReason is SUNAT catalog 10, the reason of a Nota de débito.
type DebitNoteReason string

const (
	DebitInterest      DebitNoteReason = "01" // Intereses por mora
	DebitValueIncrease DebitNoteReason = "02" // Aumento en el valor
	DebitPenalties     DebitNoteReason = "03" // Penalidades / otros conceptos
	DebitExportAdjust  DebitNoteReason = "10" // Ajustes de operaciones de exportación
	DebitIVAPAdjust    DebitNoteReason = "11" // Ajustes afectos al IVAP
	DebitPaymentFix    DebitNoteReason = "12" // Ajustes - montos y/o fechas de pago
)

// Valid reports whether r is a catalog 10 code.
func (r DebitNoteReason) Valid() bool {
	switch r {
	case DebitInterest, DebitValueIncrease, DebitPenalties, DebitExportAdjust, DebitIVAPAdjust, DebitPaymentFix:
		return true
	}
	return false
}

// RelatedDocument codes (catalog 12) used by cac:DespatchDocumentReference and
// cac:AdditionalDocumentReference.
const (
	RefDespatchSender  = "09" // Guía de remisión remitente
	RefDespatchCarrier = "31" // Guía de remisión transportista
	RefAdvanceInvoice  = "02" // Factura - emitida por anticipos
	RefAdvanceReceipt  = "03" // Boleta de venta - emitida por anticipos
)

// Detraction payment means (catalog 59) commonly used with SPOT.
const (
	PaymentMeansDeposit = "001" // Depósito en cuenta
)
