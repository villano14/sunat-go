package catalog

// DespatchType is the SUNAT catalog 01 entry of a Guía de Remisión Electrónica.
type DespatchType string

const (
	DespatchSender  DespatchType = "09" // Guía de remisión remitente (series T###)
	DespatchCarrier DespatchType = "31" // Guía de remisión transportista (series V###)
)

// TransportMode is SUNAT catalog 18 (Modalidad de traslado). It applies to
// the remitente guide (09) only.
type TransportMode string

const (
	TransportPublic  TransportMode = "01" // Transporte público
	TransportPrivate TransportMode = "02" // Transporte privado
)

// TransferReason is SUNAT catalog 20 (Motivo de traslado). It applies to the
// remitente guide (09) only.
type TransferReason string

const (
	TransferSale                   TransferReason = "01" // Venta
	TransferPurchase               TransferReason = "02" // Compra
	TransferSaleToThirdParty       TransferReason = "03" // Venta con entrega a terceros
	TransferBetweenEstablishments  TransferReason = "04" // Traslado entre establecimientos de la misma empresa
	TransferConsignment            TransferReason = "05" // Consignación
	TransferReturn                 TransferReason = "06" // Devolución
	TransferTransformedGoodsPickup TransferReason = "07" // Recojo de bienes transformados
	TransferImport                 TransferReason = "08" // Importación
	TransferExport                 TransferReason = "09" // Exportación
	TransferOther                  TransferReason = "13" // Otros
	TransferSaleSubjectToBuyerOK   TransferReason = "14" // Venta sujeta a confirmación del comprador
	TransferForTransformation      TransferReason = "17" // Traslado de bienes para transformación
	TransferItinerantIssuer        TransferReason = "18" // Traslado emisor itinerante CP
	TransferToPrimaryZone          TransferReason = "19" // Traslado a zona primaria
)

// DespatchIndicator is a shipment indicator rendered as cbc:SpecialInstructions.
// The values are the literal SUNAT identifiers Greenter documents.
type DespatchIndicator string

const (
	IndicatorScheduledTransshipment DespatchIndicator = "SUNAT_Envio_IndicadorTransbordoProgramado"
	IndicatorVehicleM1L             DespatchIndicator = "SUNAT_Envio_IndicadorTrasladoVehiculoM1L"
	IndicatorEmptyContainerReturn   DespatchIndicator = "SUNAT_Envio_IndicadorRetornoVehiculoEnvaseVacio"
	IndicatorEmptyVehicleReturn     DespatchIndicator = "SUNAT_Envio_IndicadorRetornoVehiculoVacio"
	IndicatorTotalDAMOrDS           DespatchIndicator = "SUNAT_Envio_IndicadorTrasladoTotalDAMoDS"
	IndicatorVehicleAndDrivers      DespatchIndicator = "SUNAT_Envio_IndicadorVehiculoConductoresTransp"
)

// DriverType tells the main driver from the secondary ones. SUNAT expects
// the Spanish words, rendered as cbc:JobTitle.
type DriverType string

const (
	DriverMain      DriverType = "Principal"
	DriverSecondary DriverType = "Secundario"
)

// RelatedDocType is SUNAT catalog 61 (Documento relacionado al transporte).
// The list is not exhaustive: any catalog value is accepted as a string
// conversion, e.g. catalog.RelatedDocType("80").
type RelatedDocType string

const (
	RelatedInvoice          RelatedDocType = "01" // Factura
	RelatedReceipt          RelatedDocType = "03" // Boleta de venta
	RelatedSenderGuide      RelatedDocType = "09" // Guía de remisión remitente
	RelatedCarrierGuide     RelatedDocType = "31" // Guía de remisión transportista
	RelatedCustomsDeclaring RelatedDocType = "50" // Declaración aduanera de mercancías
)
