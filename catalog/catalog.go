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
	Diplomatic   IdentityType = "A" // Cédula diplomática de identidad
	NonDomiciled IdentityType = "B" // Documento identificatorio país de residencia no domiciliado
	TaxIDForeign IdentityType = "C" // Tax Identification Number (TIN) - doc. trib. PP.NN.
	IDForeign    IdentityType = "D" // Identification Number (IN) - doc. trib. PP.JJ.
	TAMPermit    IdentityType = "E" // Tarjeta Andina de Migración (TAM)
	PTPPermit    IdentityType = "F" // Permiso Temporal de Permanencia (PTP)
	SafeConduct  IdentityType = "G" // Salvoconducto
)

// IGVAffectation is SUNAT catalog 07.
type IGVAffectation string

const (
	TaxedOnerous      IGVAffectation = "10" // Gravado - Operación onerosa
	TaxedFree         IGVAffectation = "11" // Gravado - Retiro por premio
	TaxedDonation     IGVAffectation = "12" // Gravado - Retiro por donación
	TaxedWithdrawal   IGVAffectation = "13" // Gravado - Retiro
	TaxedAdvertising  IGVAffectation = "14" // Gravado - Retiro por publicidad
	TaxedBonus        IGVAffectation = "15" // Gravado - Bonificaciones
	TaxedWorkers      IGVAffectation = "16" // Gravado - Retiro por entrega a trabajadores
	TaxedIVAP         IGVAffectation = "17" // Gravado - IVAP
	ExoneratedOnerous IGVAffectation = "20" // Exonerado - Operación onerosa
	ExoneratedFree    IGVAffectation = "21" // Exonerado - Transferencia gratuita
	UnaffectedOnerous IGVAffectation = "30" // Inafecto - Operación onerosa
	UnaffectedBonus   IGVAffectation = "31" // Inafecto - Retiro por bonificación
	UnaffectedFree    IGVAffectation = "31" // Alias of UnaffectedBonus (kept for compatibility)
	UnaffectedWithdr  IGVAffectation = "32" // Inafecto - Retiro
	UnaffectedSamples IGVAffectation = "33" // Inafecto - Retiro por muestras médicas
	UnaffectedUnion   IGVAffectation = "34" // Inafecto - Retiro por convenio colectivo
	UnaffectedPrize   IGVAffectation = "35" // Inafecto - Retiro por premio
	UnaffectedAdv     IGVAffectation = "36" // Inafecto - Retiro por publicidad
	Export            IGVAffectation = "40" // Exportación
)

// Valid reports whether a is a catalog 07 code.
func (a IGVAffectation) Valid() bool {
	_, err := SchemeFor(a)
	return err == nil
}

// Free reports whether a is a free-of-charge operation (transferencia gratuita):
// the line is priced with the reference value (catalog 16 code 02) and its
// amounts are reported under the GRA scheme.
func (a IGVAffectation) Free() bool {
	switch a {
	case TaxedFree, TaxedDonation, TaxedWithdrawal, TaxedAdvertising, TaxedBonus, TaxedWorkers,
		ExoneratedFree, UnaffectedBonus, UnaffectedWithdr, UnaffectedSamples, UnaffectedUnion,
		UnaffectedPrize, UnaffectedAdv:
		return true
	}
	return false
}

// OperationType is SUNAT catalog 51.
type OperationType string

const (
	InternalSale       OperationType = "0101" // Venta interna
	InternalSaleDeduct OperationType = "0112" // Venta interna - sustenta gastos deducibles persona natural
	InternalSaleNRUS   OperationType = "0113" // Venta interna - NRUS
	ExportGoods        OperationType = "0200" // Exportación de bienes
	ExportServices     OperationType = "0201" // Exportación de servicios - prestados íntegramente en el país
	ExportLodging      OperationType = "0202" // Exportación de servicios - hospedaje no domiciliado
	ExportTransport    OperationType = "0203" // Exportación de servicios - transporte de navieras
	ExportShips        OperationType = "0204" // Exportación de servicios - a naves y aeronaves de bandera extranjera
	ExportTourism      OperationType = "0205" // Exportación de servicios - paquete turístico
	ExportCargo        OperationType = "0206" // Exportación de servicios - complementarios al transporte de carga
	ExportEnergy       OperationType = "0207" // Exportación de servicios - suministro de energía eléctrica al exterior
	ExportComms        OperationType = "0208" // Exportación de servicios - comunicación
	AirWaybill         OperationType = "0301" // Operaciones con carta de porte aéreo
	RailPassengers     OperationType = "0302" // Operaciones de transporte ferroviario de pasajeros
	Royalties          OperationType = "0303" // Operaciones de pago de regalías petrolíferas
	NonDomiciledSale   OperationType = "0401" // Ventas no domiciliados que no califican como exportación
	Detraction         OperationType = "1001" // Operación sujeta a detracción
	DetractionFishery  OperationType = "1002" // Operación sujeta a detracción - recursos hidrobiológicos
	DetractionPassgrs  OperationType = "1003" // Operación sujeta a detracción - transporte de pasajeros
	DetractionCargo    OperationType = "1004" // Operación sujeta a detracción - transporte de carga
	PerceptionSale     OperationType = "2001" // Operación sujeta a percepción
)

// LegendCode is SUNAT catalog 52.
type LegendCode string

const (
	AmountInWords      LegendCode = "1000" // Monto en letras
	FreeTransfer       LegendCode = "1002" // Transferencia gratuita de un bien y/o servicio
	PerceptionVoucher  LegendCode = "2000" // Comprobante de percepción
	AmazonGoods        LegendCode = "2001" // Bienes transferidos en la Amazonía
	AmazonServices     LegendCode = "2002" // Servicios prestados en la Amazonía
	AmazonConstruction LegendCode = "2003" // Contratos de construcción ejecutados en la Amazonía
	TravelAgency       LegendCode = "2004" // Agencia de viaje - paquete turístico
	ItinerantSeller    LegendCode = "2005" // Venta realizada por emisor itinerante
	DetractionNote     LegendCode = "2006" // Operación sujeta a detracción
	IVAPNote           LegendCode = "2007" // Operación sujeta al IVAP
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
	case TaxedIVAP:
		return SchemeIVAP, nil
	case ExoneratedOnerous:
		return SchemeExonerated, nil
	case UnaffectedOnerous:
		return SchemeUnaffected, nil
	case Export:
		return SchemeExport, nil
	}
	if a.Free() {
		return SchemeFree, nil
	}
	return TaxScheme{}, fmt.Errorf("catalog: unsupported IGV affectation %q", string(a))
}
