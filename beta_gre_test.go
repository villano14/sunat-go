package sunat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/gre"
)

// SUNAT publishes no beta host for GRE, so this test is opt-in and needs
// everything from the environment; nothing is hardcoded:
//
//	SUNAT_GRE_BETA=1
//	SUNAT_GRE_CLIENT_ID, SUNAT_GRE_CLIENT_SECRET   (SOL menu "Credenciales de API SUNAT")
//	SUNAT_GRE_RUC, SUNAT_GRE_USER, SUNAT_GRE_PASSWORD
//	SUNAT_GRE_SECURITY_URL, SUNAT_GRE_API_URL      (optional; default production)
//
// Greenter's beta is Nubefact's homologation server, https://gre-test.nubefact.com/v1
// (gre.BetaBase), with the public test credentials listed in Greenter's demo
// project. Against production, a successful send issues a real guide.

func betaGREClient(t *testing.T) (*sunat.GREClient, string) {
	t.Helper()
	if os.Getenv("SUNAT_GRE_BETA") != "1" {
		t.Skip("set SUNAT_GRE_BETA=1 (plus SUNAT_GRE_* credentials) to send a guide to the GRE API")
	}
	if testing.Short() {
		t.Skip("SUNAT GRE integration test")
	}
	env := func(name string) string {
		t.Helper()
		v := os.Getenv(name)
		if v == "" {
			t.Fatalf("%s is required when SUNAT_GRE_BETA=1", name)
		}
		return v
	}
	creds := gre.Credentials{
		ClientID:     env("SUNAT_GRE_CLIENT_ID"),
		ClientSecret: env("SUNAT_GRE_CLIENT_SECRET"),
		RUC:          env("SUNAT_GRE_RUC"),
		User:         env("SUNAT_GRE_USER"),
		Password:     env("SUNAT_GRE_PASSWORD"),
	}
	var opts []gre.Option
	if sec, api := os.Getenv("SUNAT_GRE_SECURITY_URL"), os.Getenv("SUNAT_GRE_API_URL"); sec != "" || api != "" {
		if sec == "" || api == "" {
			t.Fatal("set both SUNAT_GRE_SECURITY_URL and SUNAT_GRE_API_URL, or neither")
		}
		opts = append(opts, gre.WithEndpoints(sec, api))
	}
	signer, _ := greSigner(t)
	return sunat.NewGRE(signer, gre.New(creds, opts...)), creds.RUC
}

func TestBetaSendDespatch(t *testing.T) {
	client, ruc := betaGREClient(t)
	for _, kind := range []catalog.DespatchType{catalog.DespatchSender, catalog.DespatchCarrier} {
		t.Run(string(kind), func(t *testing.T) {
			g := greSampleDespatch(kind)
			now := time.Now()
			g.Company.RUC = ruc
			g.IssueDate = now.In(time.FixedZone("PET", -5*3600))
			g.Number = now.Unix() % 100000000
			start := g.IssueDate
			g.Shipment.StartDate = &start

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			sent, err := client.SendDespatch(ctx, g)
			if err != nil {
				t.Fatalf("SendDespatch() error: %v", err)
			}
			t.Logf("ticket=%s", sent.Ticket.Number)

			for attempt := 0; attempt < 12; attempt++ {
				res, err := client.DespatchStatus(ctx, sent.Ticket.Number)
				if err != nil {
					t.Fatalf("DespatchStatus() error: %v", err)
				}
				if res.Status.InProcess() {
					time.Sleep(5 * time.Second)
					continue
				}
				if res.Status.Rejected() {
					t.Fatalf("SUNAT rejected the guide: %v (CDR code=%q)", res.Status.Error, res.CDR.Code)
				}
				t.Logf("CDR code=%s description=%s notes=%q", res.CDR.Code, res.CDR.Description, res.CDR.Notes)
				if !res.CDR.Accepted() {
					t.Fatal("GRE API did not accept the guide")
				}
				return
			}
			t.Fatal("ticket still in process after the polling window")
		})
	}
}
