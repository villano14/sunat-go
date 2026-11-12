package ubl_test

import (
	"strings"
	"testing"
	"time"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func samplePurchase() model.PurchaseSettlement {
	return model.PurchaseSettlement{
		OperationType: catalog.PurchaseInternal,
		Series:        "L001", Number: 7,
		IssueDate: time.Date(2026, 10, 27, 10, 30, 0, 0, batchPET),
		Currency:  "PEN",
		Company: model.Company{
			RUC: "20000000001", LegalName: "EMPRESA & COMPRADORA S.A.C.", TradeName: "COMPRADORA",
			Address: model.Address{Ubigeo: "150101", Department: "LIMA", Province: "LIMA", District: "LIMA", Line: "AV. LOS OLIVOS 123"},
		},
		Seller: model.PurchaseSeller{
			IdentityType: catalog.DNI, Number: "12345678", Name: "JUAN PEREZ <PRODUCTOR>",
			Ubigeo: "080101", Line: "CHACRA SAN JUAN S/N", AddressType: "01",
		},
		Location:     model.PurchaseLocation{Type: "02", Ubigeo: "080101", Line: "FUNDO LAS PALMAS"},
		DespatchRefs: []model.DocumentRef{{Type: "09", Number: "T001-15"}},
		VehiclePlate: "ABC123",
		Legends:      []model.Legend{{Code: catalog.AmountInWords, Value: "MIL CIENTO OCHENTA Y DOS CON 00/100 SOLES"}},

		TaxedAmount: d("1000"), ExoneratedAmount: d("200"),
		IGV: d("180"), IncomeTaxBase: d("1200"), IncomeTax: d("18"),
		LineExtension: d("1200"), TaxInclusive: d("1380"), Payable: d("1182"),
		Lines: []model.PurchaseSettlementLine{
			{
				ProductCode: "CAFE-01", SunatProductCode: "10191509", Unit: "KGM",
				Description: "CAFE EN GRANO", Quantity: d("100"),
				UnitValue: d("10"), UnitPrice: d("11.8"), LineAmount: d("1000"),
				Affectation: catalog.TaxedOnerous, IGVPercent: d("18"), IGV: d("180"),
				IncomeTaxPercent: d("1.5"), IncomeTax: d("15"),
			},
			{
				SunatProductCode: "10191510", Description: "CACAO",
				Quantity: d("20"), UnitValue: d("10"), UnitPrice: d("10"), LineAmount: d("200"),
				Affectation:      catalog.ExoneratedOnerous,
				IncomeTaxPercent: d("1.5"), IncomeTax: d("3"),
			},
		},
	}
}

func TestPurchaseSettlementGolden(t *testing.T) {
	out, err := ubl.PurchaseSettlement(samplePurchase())
	if err != nil {
		t.Fatalf("PurchaseSettlement() error: %v", err)
	}
	batchGolden(t, "purchase-basic.xml", out)
}

func TestPurchaseSettlementXSD(t *testing.T) {
	gold := samplePurchase()
	gold.OperationType = catalog.PurchaseGold
	gold.VehiclePlate, gold.DespatchRefs, gold.Legends, gold.Company.TradeName = "", nil, nil, ""
	gold.Lines[0].Attributes = model.GoldAttributes("010203", "95.5", "AURIF", "MINA SOL")
	for name, p := range map[string]model.PurchaseSettlement{"basic": samplePurchase(), "gold minimal": gold} {
		t.Run(name, func(t *testing.T) {
			out, err := ubl.PurchaseSettlement(p)
			if err != nil {
				t.Fatal(err)
			}
			validateXSD(t, "2.1/maindoc/UBL-SelfBilledInvoice-2.1.xsd", signForXSD(t, out))
		})
	}
}

func TestPurchaseSettlementValues(t *testing.T) {
	p := samplePurchase()
	if p.ID() != "L001-7" || p.FileName() != "20000000001-04-L001-7" {
		t.Fatalf("ID/FileName = %q / %q", p.ID(), p.FileName())
	}
	out, err := ubl.PurchaseSettlement(p)
	if err != nil {
		t.Fatal(err)
	}
	root := parse(t, out)
	if root.XMLName.Local != "SelfBilledInvoice" || root.XMLName.Space != "urn:oasis:names:specification:ubl:schema:xsd:SelfBilledInvoice-2" {
		t.Fatalf("root = %v", root.XMLName)
	}
	// The buyer precedes the seller, unlike an Invoice.
	var order []string
	for _, n := range root.Nodes {
		if n.XMLName.Local == "AccountingCustomerParty" || n.XMLName.Local == "AccountingSupplierParty" {
			order = append(order, n.XMLName.Local)
		}
	}
	if strings.Join(order, ",") != "AccountingCustomerParty,AccountingSupplierParty" {
		t.Errorf("party order = %v", order)
	}
	for _, c := range []struct {
		want string
		path []string
	}{
		{"L001-7", []string{"ID"}},
		{"10:30:00", []string{"IssueTime"}},
		{"04", []string{"InvoiceTypeCode"}},
		{"SIGN20000000001", []string{"Signature", "ID"}},
		{"20000000001", []string{"AccountingCustomerParty", "Party", "PartyIdentification", "ID"}},
		{"12345678", []string{"AccountingSupplierParty", "Party", "PartyIdentification", "ID"}},
		{"JUAN PEREZ <PRODUCTOR>", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationName"}},
		{"01", []string{"AccountingSupplierParty", "Party", "PartyLegalEntity", "RegistrationAddress", "AddressTypeCode"}},
		{"ABC123", []string{"Delivery", "Shipment", "ShipmentStage", "TransportMeans", "RoadTransport", "LicensePlateID"}},
		{"02", []string{"DeliveryTerms", "DeliveryLocation", "LocationTypeCode"}},
		{"180.00", []string{"TaxTotal", "TaxAmount"}},
		{"1200.00", []string{"LegalMonetaryTotal", "LineExtensionAmount"}},
		{"1380.00", []string{"LegalMonetaryTotal", "TaxInclusiveAmount"}},
		{"1182.00", []string{"LegalMonetaryTotal", "PayableAmount"}},
	} {
		if got, _ := root.find(c.path...); got != c.want {
			t.Errorf("%s = %q, want %q", strings.Join(c.path, "/"), got, c.want)
		}
	}
	// Header subtotals: IGV, EXO and the informative income tax.
	var schemes []string
	for _, tt := range batchChildren(root, "TaxTotal") {
		for _, st := range batchChildren(tt, "TaxSubtotal") {
			id, _ := st.find("TaxCategory", "TaxScheme", "ID")
			tax, _ := st.find("TaxAmount")
			schemes = append(schemes, id+"="+tax)
		}
	}
	if got := strings.Join(schemes, ","); got != "1000=180.00,9997=0.00,3000=18.00" {
		t.Errorf("header subtotals = %s", got)
	}
	lines := batchChildren(root, "InvoiceLine")
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	if u := lines[1].Nodes[1].Attrs; len(u) == 0 || u[0].Value != "KGM" {
		t.Errorf("default unit = %v, want KGM", u)
	}
	var lineSchemes []string
	for _, st := range batchChildren(batchChildren(lines[1], "TaxTotal")[0], "TaxSubtotal") {
		id, _ := st.find("TaxCategory", "TaxScheme", "ID")
		pct, _ := st.find("TaxCategory", "Percent")
		lineSchemes = append(lineSchemes, id+"@"+pct)
	}
	if got := strings.Join(lineSchemes, ","); got != "9997@0,3000@1.5" {
		t.Errorf("exonerated line subtotals = %s", got)
	}
}

func TestPurchaseSettlementDefaults(t *testing.T) {
	p := samplePurchase()
	p.OperationType, p.Currency = "", ""
	out, err := ubl.PurchaseSettlement(p)
	if err != nil {
		t.Fatal(err)
	}
	root := parse(t, out)
	if cur, _ := root.find("DocumentCurrencyCode"); cur != "PEN" {
		t.Errorf("currency = %q", cur)
	}
	if !strings.Contains(string(out), `listID="0501"`) {
		t.Error("operation type must default to 0501")
	}
}

func TestPurchaseSettlementOnlyExonerated(t *testing.T) {
	p := samplePurchase()
	p.Lines = p.Lines[1:]
	p.TaxedAmount, p.IGV = d("0"), d("0")
	out, err := ubl.PurchaseSettlement(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), ">1000</cbc:ID>") {
		t.Error("an exonerated-only settlement must not report the IGV subtotal")
	}
}

func TestPurchaseSettlementValidation(t *testing.T) {
	for name, mutate := range map[string]func(*model.PurchaseSettlement){
		"bad series":        func(p *model.PurchaseSettlement) { p.Series = "F001" },
		"lowercase series":  func(p *model.PurchaseSettlement) { p.Series = "l001" },
		"zero number":       func(p *model.PurchaseSettlement) { p.Number = 0 },
		"huge number":       func(p *model.PurchaseSettlement) { p.Number = 100000000 },
		"no issue date":     func(p *model.PurchaseSettlement) { p.IssueDate = time.Time{} },
		"bad operation":     func(p *model.PurchaseSettlement) { p.OperationType = catalog.InternalSale },
		"no company":        func(p *model.PurchaseSettlement) { p.Company.RUC = "" },
		"seller with RUC":   func(p *model.PurchaseSettlement) { p.Seller.IdentityType = catalog.RUC },
		"seller no name":    func(p *model.PurchaseSettlement) { p.Seller.Name = "" },
		"seller no ubigeo":  func(p *model.PurchaseSettlement) { p.Seller.Ubigeo = "" },
		"seller bad type":   func(p *model.PurchaseSettlement) { p.Seller.AddressType = "09" },
		"location bad type": func(p *model.PurchaseSettlement) { p.Location.Type = "" },
		"location no line":  func(p *model.PurchaseSettlement) { p.Location.Line = "" },
		"no lines":          func(p *model.PurchaseSettlement) { p.Lines = nil },
		"export line":       func(p *model.PurchaseSettlement) { p.Lines[0].Affectation = catalog.Export },
		"no description":    func(p *model.PurchaseSettlement) { p.Lines[0].Description = "" },
		"no product code":   func(p *model.PurchaseSettlement) { p.Lines[0].SunatProductCode = "" },
		"zero quantity":     func(p *model.PurchaseSettlement) { p.Lines[0].Quantity = d("0") },
		"IGV on exonerated": func(p *model.PurchaseSettlement) { p.Lines[1].IGV = d("1") },
		"empty attribute":   func(p *model.PurchaseSettlement) { p.Lines[0].Attributes = []model.ItemAttribute{{Code: "6000"}} },
		"bad despatch type": func(p *model.PurchaseSettlement) { p.DespatchRefs[0].Type = "01" },
	} {
		t.Run(name, func(t *testing.T) {
			p := samplePurchase()
			mutate(&p)
			if _, err := ubl.PurchaseSettlement(p); err == nil || !strings.HasPrefix(err.Error(), "ubl: purchase settlement") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGoldAttributes(t *testing.T) {
	got := model.GoldAttributes("010203", "", "AURIF", "")
	if len(got) != 2 || got[0].Code != "6000" || got[1].Code != "6005" || got[1].Name != "Comercialización de Oro : Naturaleza del mineral" {
		t.Errorf("GoldAttributes = %+v", got)
	}
}
