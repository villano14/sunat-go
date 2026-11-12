//go:build ignore

// Renders the same parity fixtures as greenter.php with sunat-go.
package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

var pet = time.FixedZone("PET", -5*3600)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func at(y int, m time.Month, day, h, min int) time.Time {
	return time.Date(y, m, day, h, min, 0, 0, pet)
}

func company() model.Company {
	return model.Company{
		RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C.", TradeName: "PRUEBA",
		Address: model.Address{Ubigeo: "150101", Department: "LIMA", Province: "LIMA", District: "LIMA",
			Urbanization: "URB. CENTRO", Line: "AV. LOS OLIVOS 123", CountryCode: "PE", EstablishmentCode: "0000"},
		Email: "ventas@prueba.pe", Phone: "01-5551234",
	}
}

func clientRUC() model.Client {
	return model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE & ASOCIADOS S.A.",
		Address: &model.Address{Ubigeo: "150122", Line: "CALLE LAS FLORES 456", CountryCode: "PE"}}
}

func main() {
	out := os.Args[1]
	write := func(name string, b []byte, err error) {
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	due := at(2026, 11, 26, 0, 0)
	inv := model.Invoice{
		Type: catalog.Invoice, OperationType: catalog.Detraction, Series: "F001", Number: 123,
		IssueDate: at(2026, 10, 27, 13, 5), DueDate: &due, Currency: "PEN",
		PaymentTerms: &model.PaymentTerms{Credit: true, Amount: d("1100"), Installments: []model.Installment{
			{Amount: d("600"), DueDate: at(2026, 11, 10, 0, 0)}, {Amount: d("500"), DueDate: at(2026, 11, 26, 0, 0)}}},
		Company: company(), Client: clientRUC(),
		Seller:           &model.Client{IdentityType: catalog.RUC, Number: "20999999999", Name: "VENDEDOR S.A.C."},
		Delivery:         &model.Address{Ubigeo: "150101", Department: "LIMA", Province: "LIMA", District: "LIMA", Line: "JR. ENTREGA 1", CountryCode: "PE"},
		OrderReference:   "OC-2026-55",
		DespatchRefs:     []model.DocumentRef{{Type: "09", Number: "T001-45"}},
		RelatedDocs:      []model.DocumentRef{{Type: "99", Number: "CONTRATO-9"}},
		Detraction:       &model.Detraction{MeanOfPayment: "001", BankAccount: "00-123-456789", GoodCode: "037", Percent: d("12"), Amount: d("156")},
		AllowanceCharges: []model.AllowanceCharge{{Reason: "03", Factor: d("0.01"), Amount: d("13"), BaseAmount: d("1300")}},
		AllowanceTotal:   d("13"),
		TaxedAmount:      d("1000"), ExoneratedAmount: d("100"), FreeAmount: d("50"),
		IGV: d("180"), ISCBase: d("500"), ISC: d("50"), ICBPER: d("0.5"),
		TotalTaxes: d("230.5"), LineExtension: d("1100"), TaxInclusive: d("1330.5"), Rounding: d("0.5"), Payable: d("1318"),
		Observation: "ENTREGA EN ALMACEN <2>",
		Legends: []model.Legend{{Code: catalog.AmountInWords, Value: "MIL TRESCIENTOS DIECIOCHO CON 00/100 SOLES"},
			{Code: catalog.DetractionNote, Value: "Operación sujeta a detracción"}},
		Lines: []model.InvoiceLine{
			{ProductCode: "P001", Unit: "NIU", Quantity: d("5"), Description: "PRODUCTO <UNO> & CIA",
				SunatProductCode: "43211507", GS1Code: "7750000000017", UnitValue: d("100"),
				AllowanceCharges: []model.AllowanceCharge{{Reason: "00", Factor: d("0.1"), Amount: d("50"), BaseAmount: d("500")}},
				ISCBase:          d("500"), ISCPercent: d("10"), ISC: d("50"), ISCSystem: "01",
				TaxBase: d("500"), IGVPercent: d("18"), IGV: d("90"), Affectation: catalog.TaxedOnerous, TotalTaxes: d("140"),
				UnitPrice: d("118"), LineAmount: d("450"),
				Attributes: []model.ItemAttribute{{Code: "7000", Name: "Placa", Value: "ABC-123"}}},
			{ProductCode: "BOLSA", Unit: "NIU", Quantity: d("2.5"), Description: "BOLSA PLASTICA", UnitValue: d("200"),
				TaxBase: d("500"), IGVPercent: d("18"), IGV: d("90"), Affectation: catalog.TaxedOnerous,
				ICBPER: d("0.5"), ICBPERFactor: d("0.2"), TotalTaxes: d("90.5"), UnitPrice: d("236.123456789"), LineAmount: d("500")},
			{Unit: "ZZ", Quantity: d("1"), Description: "SERVICIO EXONERADO", UnitValue: d("100"),
				TaxBase: d("100"), IGVPercent: d("0"), Affectation: catalog.ExoneratedOnerous, UnitPrice: d("100"), LineAmount: d("100")},
			{Unit: "NIU", Quantity: d("1"), Description: "MUESTRA GRATIS", UnitValue: d("0"),
				TaxBase: d("50"), IGVPercent: d("0"), Affectation: catalog.ExoneratedFree, UnitPrice: d("50"), LineAmount: d("50")},
		},
	}
	b, err := ubl.Invoice(inv)
	write("invoice-full.xml", b, err)

	bol := model.Invoice{
		Type: catalog.Receipt, OperationType: catalog.InternalSale, Series: "B001", Number: 9,
		IssueDate: at(2026, 10, 27, 9, 0), Currency: "PEN", Company: company(),
		Client:      model.Client{IdentityType: catalog.DNI, Number: "12345678", Name: "JUAN PEREZ"},
		TaxedAmount: d("100"), IGV: d("18"), TotalTaxes: d("18"), LineExtension: d("100"), TaxInclusive: d("118"), Payable: d("118"),
		Legends: []model.Legend{{Code: catalog.AmountInWords, Value: "CIENTO DIECIOCHO CON 00/100 SOLES"}},
		Lines: []model.InvoiceLine{{ProductCode: "P9", Unit: "NIU", Quantity: d("1"), Description: "ITEM", UnitValue: d("100"),
			TaxBase: d("100"), IGVPercent: d("18"), IGV: d("18"), Affectation: catalog.TaxedOnerous, TotalTaxes: d("18"),
			UnitPrice: d("118"), LineAmount: d("100")}},
	}
	b, err = ubl.Invoice(bol)
	write("boleta.xml", b, err)

	for _, n := range []struct {
		typ            catalog.DocumentType
		series, reason string
		desc           string
	}{{catalog.CreditNote, "FC01", "01", "ANULACION DE LA OPERACION"}, {catalog.DebitNote, "FD01", "02", "AUMENTO EN EL VALOR"}} {
		note := model.Note{
			Type: n.typ, Series: n.series, Number: 4, IssueDate: at(2026, 10, 28, 10, 15), Currency: "PEN",
			AffectedType: catalog.Invoice, AffectedNumber: "F001-123", Reason: n.reason, Description: n.desc,
			Company: company(), Client: clientRUC(),
			DespatchRefs: []model.DocumentRef{{Type: "09", Number: "T001-45"}},
			TaxedAmount:  d("200"), IGV: d("36"), TotalTaxes: d("36"), Payable: d("236"),
			Legends: []model.Legend{{Code: catalog.AmountInWords, Value: "DOSCIENTOS TREINTA Y SEIS CON 00/100 SOLES"}},
			Lines: []model.InvoiceLine{{ProductCode: "P001", Unit: "NIU", Quantity: d("2"), Description: "PRODUCTO", UnitValue: d("100"),
				TaxBase: d("200"), IGVPercent: d("18"), IGV: d("36"), Affectation: catalog.TaxedOnerous, TotalTaxes: d("36"),
				UnitPrice: d("118"), LineAmount: d("200")}},
		}
		if n.typ == catalog.CreditNote {
			note.PaymentTerms = &model.PaymentTerms{Credit: true, Amount: d("236"), Installments: []model.Installment{{Amount: d("236"), DueDate: at(2026, 11, 28, 0, 0)}}}
		}
		b, err = ubl.Note(note)
		write("note-"+string(n.typ)+".xml", b, err)
	}
}
