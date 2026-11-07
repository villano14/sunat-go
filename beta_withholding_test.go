package sunat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/transport"
)

// The otros CPE endpoint is a different billService from invoices, so these
// tests build their own Client with transport.OtrosCPEBeta.

func betaOtrosCPEClient(t *testing.T) *sunat.Client {
	t.Helper()
	if os.Getenv("SUNAT_BETA") != "1" {
		t.Skip("set SUNAT_BETA=1 to send a retention or perception to SUNAT beta")
	}
	if testing.Short() {
		t.Skip("SUNAT beta integration test")
	}
	signer, _ := withholdingSigner(t)
	tr := transport.New(transport.OtrosCPEBeta, transport.Credentials{
		RUC: "20000000001", User: "MODDATOS", Password: "moddatos",
	})
	return sunat.New(signer, tr)
}

func TestBetaSendRetention(t *testing.T) {
	client := betaOtrosCPEClient(t)
	ret := sampleRetention()
	now := time.Now()
	ret.IssueDate = now.In(time.FixedZone("PET", -5*3600))
	ret.Number = now.Unix() % 100000000
	for i := range ret.Details {
		ret.Details[i].IssueDate = ret.IssueDate
		ret.Details[i].RetentionDate = ret.IssueDate
		for j := range ret.Details[i].Payments {
			ret.Details[i].Payments[j].Date = ret.IssueDate
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := client.SendRetention(ctx, ret)
	if err != nil {
		t.Fatalf("SendRetention() error: %v", err)
	}
	t.Logf("CDR code=%s description=%s notes=%q", result.CDR.Code, result.CDR.Description, result.CDR.Notes)
	if !result.CDR.Accepted() {
		t.Fatal("SUNAT beta did not accept the retention")
	}
}

func TestBetaSendPerception(t *testing.T) {
	client := betaOtrosCPEClient(t)
	per := samplePerception()
	now := time.Now()
	per.IssueDate = now.In(time.FixedZone("PET", -5*3600))
	per.Number = now.Unix() % 100000000
	for i := range per.Details {
		per.Details[i].IssueDate = per.IssueDate
		per.Details[i].PerceptionDate = per.IssueDate
		for j := range per.Details[i].Collections {
			per.Details[i].Collections[j].Date = per.IssueDate
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := client.SendPerception(ctx, per)
	if err != nil {
		t.Fatalf("SendPerception() error: %v", err)
	}
	t.Logf("CDR code=%s description=%s notes=%q", result.CDR.Code, result.CDR.Description, result.CDR.Notes)
	if !result.CDR.Accepted() {
		t.Fatal("SUNAT beta did not accept the perception")
	}
}
