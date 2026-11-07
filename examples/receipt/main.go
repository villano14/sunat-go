// Command receipt prints an unsigned demo boleta. It reads no certificates or
// credentials and never submits anything to SUNAT.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/calc"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	d := decimal.RequireFromString
	line, err := calc.Line(calc.LineInput{
		Description: "PRODUCTO DE EJEMPLO", Quantity: d("2"), UnitPrice: d("118"),
	})
	if err != nil {
		return err
	}
	inv := model.Invoice{
		Type: catalog.Receipt, Series: "B001", Number: 1,
		IssueDate: time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Currency:  "PEN",
		Company: model.Company{
			RUC: "20000000001", LegalName: "EMPRESA DE EJEMPLO S.A.C.",
			Address: model.Address{
				Ubigeo: "150101", Department: "LIMA", Province: "LIMA",
				District: "LIMA", Line: "DIRECCION DE EJEMPLO 123",
			},
		},
		Client:  model.Client{IdentityType: catalog.DNI, Number: "12345678", Name: "CONSUMIDOR DE EJEMPLO"},
		Lines:   []model.InvoiceLine{line},
		Legends: []model.Legend{{
			Code: catalog.AmountInWords, Value: "SON DOSCIENTOS TREINTA Y SEIS CON 00/100 SOLES",
		}},
	}
	// Aggregation is an explicit caller choice, never a render/build/send step.
	totals, err := calc.Totals(calc.TotalsInput{Lines: inv.Lines})
	if err != nil {
		return err
	}
	inv = totals.Apply(inv)
	xml, err := ubl.Invoice(inv)
	if err != nil {
		return err
	}
	_, err = fmt.Print(string(xml))
	return err
}
