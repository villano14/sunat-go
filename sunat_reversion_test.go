package sunat_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func sampleReversion() model.Reversion {
	return model.Reversion{
		Company:        model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, batchPET),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, batchPET),
		Correlative:    1,
		Lines:          []model.VoidedLine{{Type: catalog.Retention, Series: "R001", Number: 1, Reason: "ERROR EN TASA"}},
	}
}

func TestSendReversion(t *testing.T) {
	signer, _ := batchSigner(t)
	fake := &batchFake{ticket: "1730000000001"}
	client := sunat.New(signer, fake)
	ctx := context.WithValue(context.Background(), struct{}{}, "reversion")

	got, err := client.SendReversion(ctx, sampleReversion())
	if err != nil {
		t.Fatalf("SendReversion() error: %v", err)
	}
	unsigned, err := ubl.Reversion(sampleReversion())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := signer.Sign(unsigned)
	switch {
	case got.Ticket != "1730000000001":
		t.Errorf("Ticket = %q", got.Ticket)
	case !bytes.Equal(got.XML, want) || !bytes.Equal(fake.xml, want):
		t.Error("SendReversion must send the signed ubl.Reversion")
	case fake.fileName != "20000000001-RR-20261028-1":
		t.Errorf("fileName = %q", fake.fileName)
	case fake.ctx != ctx:
		t.Error("context not propagated")
	}
}

func TestSendReversionErrors(t *testing.T) {
	signer, _ := batchSigner(t)

	bad := sampleReversion()
	bad.Lines[0].Type = catalog.Invoice
	if _, err := sunat.New(signer, &batchFake{}).SendReversion(context.Background(), bad); err == nil {
		t.Error("an invoice line must be rejected before sending")
	}

	res, err := sunat.New(signer, batchBillOnly{}).SendReversion(context.Background(), sampleReversion())
	if !errors.Is(err, sunat.ErrBatchUnsupported) || len(res.XML) == 0 {
		t.Errorf("error = %v, XML kept = %v; want ErrBatchUnsupported with XML", err, len(res.XML) > 0)
	}

	sendErr := errors.New("boom")
	res, err = sunat.New(signer, &batchFake{sendErr: sendErr}).SendReversion(context.Background(), sampleReversion())
	if !errors.Is(err, sendErr) || len(res.XML) == 0 {
		t.Errorf("error = %v; want wrapped transport error with XML", err)
	}
}
