package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
)

// DespatchLocation is a departure or arrival point of a shipment.
//
// Ubigeo and Line are required. EstablishmentCode and RUC identify an annex
// of a registered taxpayer (Greenter's codLocal and ruc); leave them empty
// for free-form addresses.
type DespatchLocation struct {
	Ubigeo            string // 6-digit INEI code
	Line              string // Full address
	EstablishmentCode string // Código de establecimiento anexo
	RUC               string // RUC the establishment belongs to
}

// DespatchCarrier is the transport company of a public-transport shipment.
type DespatchCarrier struct {
	IdentityType catalog.IdentityType
	Number       string
	Name         string
	MTCRegistry  string // Número de registro MTC
}

// DespatchVehicle is a vehicle with its authorization.
type DespatchVehicle struct {
	Plate             string
	CirculationCard   string            // Tarjeta única de circulación / certificado de habilitación vehicular
	AuthorizerCode    string            // Código de la entidad autorizadora
	AuthorizationCode string            // Número de autorización
	Secondary         []DespatchVehicle // Trailers; their own Secondary is ignored
}

// DespatchDriver is a driver of the shipment.
type DespatchDriver struct {
	Type         catalog.DriverType
	IdentityType catalog.IdentityType
	Number       string
	FirstName    string
	LastName     string
	License      string
}

// DespatchPort is a port (catalog 63) or airport (catalog 64).
type DespatchPort struct {
	Code string
	Name string
}

// DespatchShipment is the transfer data (cac:Shipment).
//
// TransferReason and TransportMode apply to the remitente guide (09) only and
// are not rendered for the transportista guide (31).
type DespatchShipment struct {
	TransferReason      catalog.TransferReason
	TransferDescription string // Descripción del motivo de traslado
	TransportMode       catalog.TransportMode

	StartDate    *time.Time // Fecha de inicio del traslado
	HandoverDate *time.Time // Fecha de entrega de bienes al transportista

	GrossWeight   decimal.Decimal // Peso bruto total
	WeightUnit    string          // UN/ECE rec 20, defaults to "KGM"
	ItemsWeight   decimal.Decimal // Peso bruto de los ítems; omitted when zero
	WeightSupport string          // Sustento de la diferencia de pesos
	Packages      int             // Número de bultos; omitted when zero

	// Transshipment adds catalog.IndicatorScheduledTransshipment to
	// Indicators when set (Greenter's indTransbordo).
	Transshipment bool
	Indicators    []catalog.DespatchIndicator

	Carrier    *DespatchCarrier // Public transport company
	Vehicle    *DespatchVehicle
	Drivers    []DespatchDriver
	Departure  DespatchLocation // Punto de partida
	Arrival    DespatchLocation // Punto de llegada
	Containers []string         // Container seals (precintos), one per line
	Port       *DespatchPort
	Airport    *DespatchPort
}

// DespatchRelatedDoc is a document related to the transport, such as the
// invoice that justifies the shipment.
type DespatchRelatedDoc struct {
	Type        catalog.RelatedDocType
	Description string // Human-readable document type
	Number      string
	IssuerRUC   string // Optional
}

// DespatchAttribute is an additional item property.
type DespatchAttribute struct {
	Name  string
	Code  string
	Value string // Optional
}

// DespatchLine is one shipped item.
type DespatchLine struct {
	ProductCode string
	Unit        string // UN/ECE rec 20, e.g. "NIU", "ZZ"
	Description string
	Quantity    decimal.Decimal
	SunatCode   string // Código de producto SUNAT (UNSPSC); optional
	Attributes  []DespatchAttribute
}

// Despatch is a Guía de Remisión Electrónica: remitente (09) or
// transportista (31).
//
// Company is the issuer: the remitente for 09 and the transportista for 31.
// For 31, Shipper (the remitente) is required.
type Despatch struct {
	Type      catalog.DespatchType
	Series    string // T001 for 09, V001 for 31
	Number    int64
	IssueDate time.Time
	Note      string

	Company   Company
	Recipient Client  // Destinatario
	Shipper   *Client // Remitente; required for 31, ignored for 09
	Supplier  *Client // Proveedor (Greenter's tercero); 09 only
	Buyer     *Client // Comprador

	Shipment    DespatchShipment
	RelatedDocs []DespatchRelatedDoc
	Lines       []DespatchLine
}

// ID returns the document identifier, e.g. "T001-123".
func (d Despatch) ID() string {
	return d.Series + "-" + strconv.FormatInt(d.Number, 10)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-09-T001-1".
func (d Despatch) FileName() string {
	return d.Company.RUC + "-" + string(d.Type) + "-" + d.ID()
}
