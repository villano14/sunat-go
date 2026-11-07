package sunat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/transport"
)

// TestBetaSendSummary sends a Resumen diario with one boleta to SUNAT beta and
// polls getStatus until SUNAT finishes the ticket.
//
// getStatusCdr is not exercised: SUNAT publishes no beta consult service.
func TestBetaSendSummary(t *testing.T) {
	if os.Getenv("SUNAT_BETA") != "1" {
		t.Skip("set SUNAT_BETA=1 to send a summary to SUNAT beta")
	}
	if testing.Short() {
		t.Skip("SUNAT beta integration test")
	}
	signer, _ := batchSigner(t)
	tr := transport.New(transport.BillBeta, transport.Credentials{
		RUC: "20000000001", User: "MODDATOS", Password: "moddatos",
	})
	client := sunat.New(signer, tr)

	d := decimal.RequireFromString
	now := time.Now().In(time.FixedZone("PET", -5*3600))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	summary := model.Summary{
		Company:        model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		GenerationDate: today,
		IssueDate:      today,
		Correlative:    int(now.Unix()%99999) + 1,
		Currency:       "PEN",
		Lines: []model.SummaryLine{{
			Type: catalog.Receipt, Series: "B001", Number: now.Unix() % 100000000,
			ClientIdentityType: catalog.DNI, ClientNumber: "12345678",
			Status: catalog.SummaryAdd,
			Total:  d("118"), TaxedAmount: d("100"), IGV: d("18"), IGVPercent: d("18"),
		}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sent, err := client.SendSummary(ctx, summary)
	if err != nil {
		t.Fatalf("SendSummary() error: %v", err)
	}
	t.Logf("%s ticket=%s", summary.FileName(), sent.Ticket)

	for {
		res, err := client.Status(ctx, sent.Ticket)
		if err != nil {
			t.Fatalf("Status() error: %v", err)
		}
		t.Logf("state=%s statusCode=%s code=%s description=%q", res.State, res.StatusCode, res.Code, res.Description)
		if res.InProcess() {
			select {
			case <-ctx.Done():
				t.Fatalf("ticket still in process: %v", ctx.Err())
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if !res.Accepted() {
			t.Fatalf("SUNAT beta did not accept the summary: %s: %s", res.Code, res.Description)
		}
		return
	}
}
