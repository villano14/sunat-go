package ubl_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func despatchDate(day int) *time.Time {
	t := time.Date(2026, 11, day, 0, 0, 0, 0, time.FixedZone("PET", -5*3600))
	return &t
}

// sampleDespatch09 is a remitente guide with public transport.
func sampleDespatch09() model.Despatch {
	return model.Despatch{
		Type:      catalog.DespatchSender,
		Series:    "T001",
		Number:    1,
		IssueDate: time.Date(2026, 11, 5, 9, 30, 0, 0, time.FixedZone("PET", -5*3600)),
		Note:      "ENTREGA <URGENTE> & FRAGIL",
		Company:   model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		Recipient: model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE & ASOCIADOS S.A."},
		Supplier:  &model.Client{IdentityType: catalog.RUC, Number: "20555555551", Name: "PROVEEDOR S.A."},
		Buyer:     &model.Client{IdentityType: catalog.DNI, Number: "44004400", Name: "JUAN PEREZ"},
		Shipment: model.DespatchShipment{
			TransferReason:      catalog.TransferSale,
			TransferDescription: "VENTA",
			TransportMode:       catalog.TransportPublic,
			StartDate:           despatchDate(6),
			GrossWeight:         d("12.5"),
			Packages:            3,
			Carrier: &model.DespatchCarrier{
				IdentityType: catalog.RUC,
				Number:       "20987654321",
				Name:         "TRANSPORTES RAPIDOS S.A.C.",
				MTCRegistry:  "0000001",
			},
			Departure: model.DespatchLocation{Ubigeo: "150101", Line: "AV. LOS OLIVOS 123"},
			Arrival:   model.DespatchLocation{Ubigeo: "150203", Line: "AV. ITALIA 456", EstablishmentCode: "0001", RUC: "20123456789"},
		},
		RelatedDocs: []model.DespatchRelatedDoc{{
			Type: catalog.RelatedInvoice, Description: "Factura", Number: "F001-1", IssuerRUC: "20000000001",
		}},
		Lines: []model.DespatchLine{{
			ProductCode: "P001",
			Unit:        "NIU",
			Description: "PRODUCTO <UNO>",
			Quantity:    d("2.5"),
			SunatCode:   "10101501",
			Attributes:  []model.DespatchAttribute{{Name: "Lote", Code: "7000", Value: "L-1"}},
		}},
	}
}

// sampleDespatch31 is a transportista guide issued by the carrier.
func sampleDespatch31() model.Despatch {
	return model.Despatch{
		Type:      catalog.DespatchCarrier,
		Series:    "V001",
		Number:    7,
		IssueDate: time.Date(2026, 11, 5, 9, 30, 0, 0, time.FixedZone("PET", -5*3600)),
		Company:   model.Company{RUC: "20987654321", LegalName: "TRANSPORTES RAPIDOS S.A.C."},
		Recipient: model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE & ASOCIADOS S.A."},
		Shipper:   &model.Client{IdentityType: catalog.RUC, Number: "20000000001", Name: "EMPRESA REMITENTE S.A.C."},
		Shipment: model.DespatchShipment{
			StartDate:   despatchDate(6),
			GrossWeight: d("1000"),
			WeightUnit:  "KGM",
			Carrier: &model.DespatchCarrier{
				IdentityType: catalog.RUC,
				Number:       "20987654321",
				Name:         "TRANSPORTES RAPIDOS S.A.C.",
				MTCRegistry:  "0000001",
			},
			Vehicle: &model.DespatchVehicle{Plate: "ABC123"},
			Drivers: []model.DespatchDriver{{
				Type:         catalog.DriverMain,
				IdentityType: catalog.DNI,
				Number:       "44004400",
				FirstName:    "ROBERTO",
				LastName:     "RODRIGUEZ VALENCIA",
				License:      "Q12345678",
			}},
			Departure: model.DespatchLocation{Ubigeo: "150101", Line: "AV. LOS OLIVOS 123"},
			Arrival:   model.DespatchLocation{Ubigeo: "150203", Line: "AV. ITALIA 456"},
		},
		Lines: []model.DespatchLine{{
			ProductCode: "P001", Unit: "ZZ", Description: "CARGA GENERAL", Quantity: d("10"),
		}},
	}
}

type despatchWant struct {
	value string
	path  []string
}

func dw(value string, path ...string) despatchWant { return despatchWant{value, path} }

func assertDespatchValues(t *testing.T, root node, want []despatchWant) {
	t.Helper()
	for _, w := range want {
		value, path := w.value, w.path
		got, ok := root.find(path...)
		if !ok {
			t.Errorf("missing %s", strings.Join(path, "/"))
			continue
		}
		if got != value {
			t.Errorf("%s = %q, want %q", strings.Join(path, "/"), got, value)
		}
	}
}

func TestDespatchSenderValues(t *testing.T) {
	out, err := ubl.Despatch(sampleDespatch09())
	if err != nil {
		t.Fatalf("Despatch() error: %v", err)
	}
	root := parse(t, out)
	if root.XMLName.Local != "DespatchAdvice" || root.XMLName.Space != "urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2" {
		t.Fatalf("root = %v", root.XMLName)
	}
	assertDespatchValues(t, root, []despatchWant{
		dw("2.1", "UBLVersionID"),
		dw("2.0", "CustomizationID"),
		dw("T001-1", "ID"),
		dw("2026-11-05", "IssueDate"),
		dw("09:30:00", "IssueTime"),
		dw("09", "DespatchAdviceTypeCode"),
		dw("ENTREGA <URGENTE> & FRAGIL", "Note"),
		dw("F001-1", "AdditionalDocumentReference", "ID"),
		dw("01", "AdditionalDocumentReference", "DocumentTypeCode"),
		dw("20000000001", "AdditionalDocumentReference", "IssuerParty", "PartyIdentification", "ID"),
		dw("SIGN20000000001", "Signature", "ID"),
		dw("#SUNATGO-SIGN", "Signature", "DigitalSignatureAttachment", "ExternalReference", "URI"),
		dw("EMPRESA DE PRUEBA S.A.C.", "DespatchSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"),
		dw("20123456789", "DeliveryCustomerParty", "Party", "PartyIdentification", "ID"),
		dw("CLIENTE & ASOCIADOS S.A.", "DeliveryCustomerParty", "Party", "PartyLegalEntity", "RegistrationName"),
		dw("44004400", "BuyerCustomerParty", "Party", "PartyIdentification", "ID"),
		dw("PROVEEDOR S.A.", "SellerSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"),
		dw("SUNAT_Envio", "Shipment", "ID"),
		dw("01", "Shipment", "HandlingCode"),
		dw("VENTA", "Shipment", "HandlingInstructions"),
		dw("12.500", "Shipment", "GrossWeightMeasure"),
		dw("3", "Shipment", "TotalTransportHandlingUnitQuantity"),
		dw("2026-11-06", "Shipment", "ShipmentStage", "TransitPeriod", "StartDate"),
		dw("20987654321", "Shipment", "ShipmentStage", "CarrierParty", "PartyIdentification", "ID"),
		dw("0000001", "Shipment", "ShipmentStage", "CarrierParty", "PartyLegalEntity", "CompanyID"),
		dw("150203", "Shipment", "Delivery", "DeliveryAddress", "ID"),
		dw("0001", "Shipment", "Delivery", "DeliveryAddress", "AddressTypeCode"),
		dw("AV. ITALIA 456", "Shipment", "Delivery", "DeliveryAddress", "AddressLine", "Line"),
		dw("150101", "Shipment", "Delivery", "Despatch", "DespatchAddress", "ID"),
		dw("AV. LOS OLIVOS 123", "Shipment", "Delivery", "Despatch", "DespatchAddress", "AddressLine", "Line"),
		dw("1", "DespatchLine", "ID"),
		dw("2.5", "DespatchLine", "DeliveredQuantity"),
		dw("PRODUCTO <UNO>", "DespatchLine", "Item", "Description"),
		dw("P001", "DespatchLine", "Item", "SellersItemIdentification", "ID"),
		dw("10101501", "DespatchLine", "Item", "CommodityClassification", "ItemClassificationCode"),
		dw("Lote", "DespatchLine", "Item", "AdditionalItemProperty", "Name"),
		dw("7000", "DespatchLine", "Item", "AdditionalItemProperty", "NameCode"),
		dw("L-1", "DespatchLine", "Item", "AdditionalItemProperty", "Value"),
	})
	for _, s := range []string{
		`<cbc:TransportModeCode listName="Modalidad de traslado" listAgencyName="PE:SUNAT" listURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo18">01</cbc:TransportModeCode>`,
		`<cbc:GrossWeightMeasure unitCode="KGM">12.500</cbc:GrossWeightMeasure>`,
		`<cbc:AddressTypeCode listID="20123456789">0001</cbc:AddressTypeCode>`,
		`<cbc:DeliveredQuantity unitCode="NIU">2.5</cbc:DeliveredQuantity>`,
		`<ext:ExtensionContent/>`,
	} {
		if !bytes.Contains(out, []byte(s)) {
			t.Errorf("missing %s", s)
		}
	}
	for _, s := range []string{"DespatchParty", "NetWeightMeasure", "TransportEquipment", "FirstArrivalPortLocation", "SpecialInstructions", "DriverPerson"} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("unexpected %s in a minimal public-transport guide", s)
		}
	}
}

func TestDespatchCarrierValues(t *testing.T) {
	out, err := ubl.Despatch(sampleDespatch31())
	if err != nil {
		t.Fatalf("Despatch() error: %v", err)
	}
	root := parse(t, out)
	assertDespatchValues(t, root, []despatchWant{
		dw("V001-7", "ID"),
		dw("31", "DespatchAdviceTypeCode"),
		dw("SIGN20987654321", "Signature", "ID"),
		dw("TRANSPORTES RAPIDOS S.A.C.", "DespatchSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"),
		dw("20000000001", "Shipment", "Delivery", "Despatch", "DespatchParty", "PartyIdentification", "ID"),
		dw("EMPRESA REMITENTE S.A.C.", "Shipment", "Delivery", "Despatch", "DespatchParty", "PartyLegalEntity", "RegistrationName"),
		dw("1000.000", "Shipment", "GrossWeightMeasure"),
		dw("ABC123", "Shipment", "TransportHandlingUnit", "TransportEquipment", "ID"),
		dw("44004400", "Shipment", "ShipmentStage", "DriverPerson", "ID"),
		dw("ROBERTO", "Shipment", "ShipmentStage", "DriverPerson", "FirstName"),
		dw("RODRIGUEZ VALENCIA", "Shipment", "ShipmentStage", "DriverPerson", "FamilyName"),
		dw("Principal", "Shipment", "ShipmentStage", "DriverPerson", "JobTitle"),
		dw("Q12345678", "Shipment", "ShipmentStage", "DriverPerson", "IdentityDocumentReference", "ID"),
		dw("0000001", "Shipment", "ShipmentStage", "CarrierParty", "PartyLegalEntity", "CompanyID"),
	})
	for _, s := range []string{"HandlingCode", "TransportModeCode", "SellerSupplierParty"} {
		if bytes.Contains(out, []byte(s)) {
			t.Errorf("%s must not be rendered for type 31", s)
		}
	}
}

func TestDespatchGolden(t *testing.T) {
	for name, d := range map[string]model.Despatch{
		"despatch-basic.xml":         sampleDespatch09(),
		"despatch-transportista.xml": sampleDespatch31(),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := ubl.Despatch(d)
			if err != nil {
				t.Fatalf("Despatch() error: %v", err)
			}
			golden := filepath.Join("testdata", name)
			if *update {
				if err := os.WriteFile(golden, out, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if !bytes.Equal(out, want) {
				t.Errorf("output differs from %s; run go test ./ubl -update after review\n%s", golden, out)
			}
		})
	}
}

func TestDespatchOptionalElements(t *testing.T) {
	g := sampleDespatch09()
	g.Shipment.TransportMode = catalog.TransportPrivate
	g.Shipment.Carrier = nil
	g.Shipment.HandoverDate = despatchDate(5)
	g.Shipment.ItemsWeight = d("11.25")
	g.Shipment.WeightSupport = "DIFERENCIA POR EMBALAJE"
	g.Shipment.WeightUnit = "TNE"
	g.Shipment.Transshipment = true
	g.Shipment.Indicators = []catalog.DespatchIndicator{catalog.IndicatorVehicleM1L}
	g.Shipment.Containers = []string{"PRE-1", "PRE-2"}
	g.Shipment.Port = &model.DespatchPort{Code: "PECLL", Name: "CALLAO"}
	g.Shipment.Airport = &model.DespatchPort{Code: "LIM", Name: "JORGE CHAVEZ"}
	g.Shipment.Vehicle = &model.DespatchVehicle{
		Plate: "ABC123", CirculationCard: "TUC-1", AuthorizerCode: "06", AuthorizationCode: "AUT-1",
		Secondary: []model.DespatchVehicle{{Plate: "XYZ987", CirculationCard: "TUC-2", AuthorizerCode: "06", AuthorizationCode: "AUT-2"}},
	}
	g.Shipment.Drivers = []model.DespatchDriver{
		{Type: catalog.DriverMain, IdentityType: catalog.DNI, Number: "1", FirstName: "A", LastName: "B", License: "L1"},
		{Type: catalog.DriverSecondary, IdentityType: catalog.DNI, Number: "2", FirstName: "C", LastName: "D", License: "L2"},
	}
	out, err := ubl.Despatch(g)
	if err != nil {
		t.Fatalf("Despatch() error: %v", err)
	}
	parse(t, out)
	for _, s := range []string{
		`<cbc:TransportModeCode listName="Modalidad de traslado" listAgencyName="PE:SUNAT" listURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo18">02</cbc:TransportModeCode>`,
		`<cbc:GrossWeightMeasure unitCode="TNE">12.500</cbc:GrossWeightMeasure>`,
		`<cbc:NetWeightMeasure unitCode="KGM">11.250</cbc:NetWeightMeasure>`,
		`<cbc:Information>DIFERENCIA POR EMBALAJE</cbc:Information>`,
		`<cbc:SpecialInstructions>SUNAT_Envio_IndicadorTrasladoVehiculoM1L</cbc:SpecialInstructions>`,
		`<cbc:SpecialInstructions>SUNAT_Envio_IndicadorTransbordoProgramado</cbc:SpecialInstructions>`,
		`<cbc:OccurrenceDate>2026-11-05</cbc:OccurrenceDate>`,
		`<cbc:TraceID>PRE-1</cbc:TraceID>`,
		`<cbc:TraceID>PRE-2</cbc:TraceID>`,
		`<cbc:RegistrationNationalityID>TUC-1</cbc:RegistrationNationalityID>`,
		`<cbc:ID>XYZ987</cbc:ID>`,
		`<cbc:ID schemeID="06" schemeName="Entidad Autorizadora" schemeAgencyName="PE:SUNAT">AUT-2</cbc:ID>`,
		`<cbc:ID schemeID="06" schemeName="Entidad Autorizadora" schemeAgencyName="PE:SUNAT">AUT-1</cbc:ID>`,
		`<cbc:JobTitle>Secundario</cbc:JobTitle>`,
		`<cbc:ID schemeAgencyName="PE:SUNAT" schemeName="Puertos" schemeURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo63">PECLL</cbc:ID>`,
		`<cbc:LocationTypeCode>1</cbc:LocationTypeCode>`,
	} {
		if !bytes.Contains(out, []byte(s)) {
			t.Errorf("missing %s", s)
		}
	}
	if bytes.Contains(out, []byte("Aeropuertos")) {
		t.Error("airport must not render when a port is set")
	}
	if n := strings.Count(string(out), "SUNAT_Envio_IndicadorTransbordoProgramado"); n != 1 {
		t.Errorf("transshipment indicator rendered %d times, want 1", n)
	}

	g.Shipment.Port = nil
	out, err = ubl.Despatch(g)
	if err != nil {
		t.Fatalf("Despatch() error: %v", err)
	}
	if !bytes.Contains(out, []byte(`<cbc:LocationTypeCode>2</cbc:LocationTypeCode>`)) {
		t.Error("airport not rendered")
	}

	g.Shipment.Indicators = []catalog.DespatchIndicator{catalog.IndicatorScheduledTransshipment}
	out, err = ubl.Despatch(g)
	if err != nil {
		t.Fatalf("Despatch() error: %v", err)
	}
	if n := strings.Count(string(out), "SUNAT_Envio_IndicadorTransbordoProgramado"); n != 1 {
		t.Errorf("duplicate transshipment indicator rendered %d times, want 1", n)
	}
}

func TestDespatchSecondarySecondaryIgnored(t *testing.T) {
	g := sampleDespatch31()
	g.Shipment.Vehicle = &model.DespatchVehicle{
		Plate: "ABC123",
		Secondary: []model.DespatchVehicle{{
			Plate:     "XYZ987",
			Secondary: []model.DespatchVehicle{{Plate: "NESTED1"}},
		}},
	}
	out, err := ubl.Despatch(g)
	if err != nil {
		t.Fatalf("Despatch() error: %v", err)
	}
	if bytes.Contains(out, []byte("NESTED1")) {
		t.Error("nested secondary vehicles must not be rendered")
	}
}

func TestDespatchDefaultsWeightUnit(t *testing.T) {
	out, err := ubl.Despatch(sampleDespatch09())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`unitCode="KGM">12.500<`)) {
		t.Errorf("weight unit should default to KGM:\n%s", out)
	}
}

func TestDespatchRejectsInvalid(t *testing.T) {
	bad := sampleDespatch09()
	bad.Type = catalog.DespatchType("01")
	noShipper := sampleDespatch31()
	noShipper.Shipper = nil
	for name, g := range map[string]model.Despatch{
		"unsupported type":     bad,
		"31 without remitente": noShipper,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ubl.Despatch(g); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDespatchEscapesText(t *testing.T) {
	g := sampleDespatch09()
	g.Lines[0].Description = "A ]]> B & <C>"
	out, err := ubl.Despatch(g)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := parse(t, out).find("DespatchLine", "Item", "Description")
	if got != "A ]]> B & <C>" {
		t.Errorf("Description = %q", got)
	}
}
