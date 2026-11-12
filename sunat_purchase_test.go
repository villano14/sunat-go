package sunat_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/calc"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func samplePurchase(t *testing.T) model.PurchaseSettlement {
	t.Helper()
	d := decimal.RequireFromString
	r, err := calc.Settlement(calc.SettlementInput{
		IncomeTaxPercent: d("1.5"),
		Lines: []calc.SettlementLineInput{{
			SunatProductCode: "10191509", Description: "CAFE EN GRANO", Quantity: d("100"), UnitValue: d("10"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r.Apply(model.PurchaseSettlement{
		Series: "L001", Number: 1,
		IssueDate: time.Date(2026, 10, 27, 10, 0, 0, 0, batchPET),
		Company:   model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		Seller: model.PurchaseSeller{
			IdentityType: catalog.DNI, Number: "12345678", Name: "JUAN PEREZ",
			Ubigeo: "080101", Line: "CHACRA S/N", AddressType: "01",
		},
		Location: model.PurchaseLocation{Type: "01", Ubigeo: "080101", Line: "CHACRA S/N"},
	})
}

// purchaseFake records sendBill and answers with a CDR.
type purchaseFake struct {
	fileName string
	xml      []byte
	cdr      []byte
	err      error
}

func (f *purchaseFake) SendBill(_ context.Context, fileName string, signedXML []byte) ([]byte, error) {
	f.fileName, f.xml = fileName, bytes.Clone(signedXML)
	return f.cdr, f.err
}

func TestSendPurchaseSettlement(t *testing.T) {
	signer, _ := batchSigner(t)
	p := samplePurchase(t)
	fake := &purchaseFake{cdr: batchCDR(t, "L001-1", "0", "La Liquidacion de compra numero L001-1, ha sido aceptada")}

	res, err := sunat.New(signer, fake).SendPurchaseSettlement(context.Background(), p)
	if err != nil {
		t.Fatalf("SendPurchaseSettlement() error: %v", err)
	}
	unsigned, err := ubl.PurchaseSettlement(p)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := signer.Sign(unsigned)
	switch {
	case fake.fileName != "20000000001-04-L001-1":
		t.Errorf("fileName = %q", fake.fileName)
	case !bytes.Equal(res.XML, want) || !bytes.Equal(fake.xml, want):
		t.Error("must send the signed ubl.PurchaseSettlement")
	case !res.CDR.Accepted():
		t.Errorf("CDR = %+v, want accepted", res.CDR)
	}
}

func TestSendPurchaseSettlementErrors(t *testing.T) {
	signer, _ := batchSigner(t)
	bad := samplePurchase(t)
	bad.Series = "F001"
	if _, err := sunat.New(signer, &purchaseFake{}).SendPurchaseSettlement(context.Background(), bad); err == nil {
		t.Error("invalid settlement must fail before sending")
	}
	sendErr := errors.New("boom")
	res, err := sunat.New(signer, &purchaseFake{err: sendErr}).SendPurchaseSettlement(context.Background(), samplePurchase(t))
	if !errors.Is(err, sendErr) || len(res.XML) == 0 {
		t.Errorf("error = %v; want wrapped transport error with XML kept", err)
	}
	res, err = sunat.New(signer, &purchaseFake{cdr: []byte("not a zip")}).SendPurchaseSettlement(context.Background(), samplePurchase(t))
	if err == nil || len(res.XML) == 0 {
		t.Errorf("bad CDR: error = %v, XML kept = %v", err, len(res.XML) > 0)
	}
}
