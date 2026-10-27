package sunat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/transport"
)

func TestBetaSendInvoice(t *testing.T) {
	if os.Getenv("SUNAT_BETA") != "1" {
		t.Skip("set SUNAT_BETA=1 to send an invoice to SUNAT beta")
	}
	if testing.Short() {
		t.Skip("SUNAT beta integration test")
	}
	signer, _ := newSigner(t)
	tr := transport.New(transport.BillBeta, transport.Credentials{
		RUC: "20000000001", User: "MODDATOS", Password: "moddatos",
	})
	inv := sampleInvoice()
	now := time.Now()
	inv.IssueDate = now.In(time.FixedZone("PET", -5*3600))
	inv.Number = now.Unix() % 100000000
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := sunat.New(signer, tr).SendInvoice(ctx, inv)
	if err != nil {
		t.Fatalf("SendInvoice() error: %v", err)
	}
	t.Logf("CDR code=%s description=%s notes=%q", result.CDR.Code, result.CDR.Description, result.CDR.Notes)
	if !result.CDR.Accepted() {
		t.Fatal("SUNAT beta did not accept the invoice")
	}
}
