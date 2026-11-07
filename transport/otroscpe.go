package transport

// Endpoints of the "otros CPE" billService, the one SUNAT uses for
// Comprobante de Retención (20) and Comprobante de Percepción (40). It speaks
// the same sendBill protocol as BillBeta and BillProduction, so New works
// unchanged; a Client sends to one endpoint, so retentions and perceptions
// need their own Client built with one of these.
const (
	OtrosCPEBeta       = "https://e-beta.sunat.gob.pe/ol-ti-itemision-otroscpe-gem-beta/billService"
	OtrosCPEProduction = "https://e-factura.sunat.gob.pe/ol-ti-itemision-otroscpe-gem/billService"
)
