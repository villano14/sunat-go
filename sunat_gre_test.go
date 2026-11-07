package sunat_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/gre"
	"github.com/villano14/sunat-go/internal/testcert"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/sign"
	"github.com/villano14/sunat-go/transport"
	"github.com/villano14/sunat-go/ubl"
)

func greSampleDespatch(kind catalog.DespatchType) model.Despatch {
	d := decimal.RequireFromString
	g := model.Despatch{
		Type:      kind,
		Series:    "T001",
		Number:    1,
		IssueDate: time.Date(2026, 11, 5, 9, 30, 0, 0, time.FixedZone("PET", -5*3600)),
		Company:   model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		Recipient: model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE S.A.C."},
		Shipment: model.DespatchShipment{
			TransferReason: catalog.TransferSale,
			TransportMode:  catalog.TransportPrivate,
			GrossWeight:    d("12.5"),
			Vehicle:        &model.DespatchVehicle{Plate: "ABC123"},
			Drivers: []model.DespatchDriver{{
				Type: catalog.DriverMain, IdentityType: catalog.DNI, Number: "44004400",
				FirstName: "ROBERTO", LastName: "RODRIGUEZ", License: "Q12345678",
			}},
			Departure: model.DespatchLocation{Ubigeo: "150101", Line: "AV. LOS OLIVOS 123"},
			Arrival:   model.DespatchLocation{Ubigeo: "150203", Line: "AV. ITALIA 456"},
		},
		Lines: []model.DespatchLine{{ProductCode: "P001", Unit: "NIU", Description: "PRODUCTO UNO", Quantity: d("2")}},
	}
	if kind == catalog.DespatchCarrier {
		g.Series = "V001"
		g.Shipper = &model.Client{IdentityType: catalog.RUC, Number: "20555555551", Name: "REMITENTE S.A."}
	}
	return g
}

func greSigner(t *testing.T) (*sign.XMLSigner, []byte) {
	t.Helper()
	key, cert := testcert.New(t)
	return sign.New(sign.Certificate{Key: key, Cert: cert}), testcert.CertPEM(cert)
}

type greSignerFunc func([]byte) ([]byte, error)

func (f greSignerFunc) Sign(doc []byte) ([]byte, error) { return f(doc) }

// greFakeTransport is a gre.Transport that never leaves the process.
type greFakeTransport struct {
	sendCalls   int
	statusCalls int
	ctx         context.Context
	fileName    string
	xml         []byte
	ticketArg   string

	ticket    gre.Ticket
	sendErr   error
	status    gre.Status
	statusErr error
}

func (f *greFakeTransport) Send(ctx context.Context, fileName string, signedXML []byte) (gre.Ticket, error) {
	f.sendCalls++
	f.ctx, f.fileName, f.xml = ctx, fileName, bytes.Clone(signedXML)
	return f.ticket, f.sendErr
}

func (f *greFakeTransport) Status(ctx context.Context, ticket string) (gre.Status, error) {
	f.statusCalls++
	f.ctx, f.ticketArg = ctx, ticket
	return f.status, f.statusErr
}

func greCDR(t *testing.T, fileName, code, description string) (zipped, xml []byte) {
	t.Helper()
	xml = []byte(`<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"><cbc:ID>CDR-9</cbc:ID><cbc:Note>Observation</cbc:Note><cac:DocumentResponse><cac:Response><cbc:ReferenceID>T001-1</cbc:ReferenceID><cbc:ResponseCode>` + code + `</cbc:ResponseCode><cbc:Description>` + description + `</cbc:Description></cac:Response></cac:DocumentResponse></ApplicationResponse>`)
	zipped, err := transport.Zip("R-"+fileName+".xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	return zipped, xml
}

func TestSendDespatch(t *testing.T) {
	for _, kind := range []catalog.DespatchType{catalog.DespatchSender, catalog.DespatchCarrier} {
		t.Run(string(kind), func(t *testing.T) {
			g := greSampleDespatch(kind)
			signer, _ := greSigner(t)
			tr := &greFakeTransport{ticket: gre.Ticket{Number: "ticket-1", ReceivedAt: time.Unix(1700000000, 0)}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result, err := sunat.NewGRE(signer, tr).SendDespatch(ctx, g)
			if err != nil {
				t.Fatalf("SendDespatch() error: %v", err)
			}
			wantName := "20000000001-" + string(kind) + "-" + g.Series + "-1"
			if tr.sendCalls != 1 || tr.fileName != wantName || tr.ctx != ctx {
				t.Errorf("transport calls=%d fileName=%q (want %q) context passed through=%v", tr.sendCalls, tr.fileName, wantName, tr.ctx == ctx)
			}
			if len(result.XML) == 0 || !bytes.Equal(result.XML, tr.xml) || !bytes.Contains(result.XML, []byte("<ds:Signature ")) {
				t.Error("Result.XML must be the signed XML sent to the transport")
			}
			if result.Ticket != tr.ticket {
				t.Errorf("Ticket = %+v, want %+v", result.Ticket, tr.ticket)
			}
		})
	}
}

func TestSendDespatchErrorsKeepXML(t *testing.T) {
	signer, _ := greSigner(t)
	apiErr := &gre.Error{Code: "1033", Message: "El comprobante fue registrado previamente", HTTPStatus: 422}
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"SUNAT error", apiErr},
		{"canceled context", context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := &greFakeTransport{sendErr: tt.err}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.err == context.Canceled {
				cancel()
			}
			result, err := sunat.NewGRE(signer, tr).SendDespatch(ctx, greSampleDespatch(catalog.DespatchSender))
			if err == nil || !strings.HasPrefix(err.Error(), "sunat: send despatch: ") || !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want wrapped %v", err, tt.err)
			}
			if tt.err == apiErr {
				var got *gre.Error
				if !errors.As(err, &got) || got != apiErr {
					t.Errorf("error = %v, want original *gre.Error", err)
				}
			}
			if tr.sendCalls != 1 || len(result.XML) == 0 || !bytes.Equal(result.XML, tr.xml) {
				t.Error("send errors must preserve the signed XML")
			}
			if result.Ticket != (gre.Ticket{}) {
				t.Errorf("Ticket = %+v, want zero", result.Ticket)
			}
		})
	}
}

func TestDespatchStatus(t *testing.T) {
	g := greSampleDespatch(catalog.DespatchSender)
	zipped, cdrXML := greCDR(t, g.FileName(), "0", "La Guia de remision T001-1 ha sido aceptada")
	signer, _ := greSigner(t)

	t.Run("accepted", func(t *testing.T) {
		tr := &greFakeTransport{status: gre.Status{Code: gre.CodeAccepted, CDRZip: zipped, CDRGenerated: true}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		res, err := sunat.NewGRE(signer, tr).DespatchStatus(ctx, "ticket-1")
		if err != nil {
			t.Fatalf("DespatchStatus() error: %v", err)
		}
		if tr.statusCalls != 1 || tr.ticketArg != "ticket-1" || tr.ctx != ctx {
			t.Errorf("calls=%d ticket=%q ctx passed=%v", tr.statusCalls, tr.ticketArg, tr.ctx == ctx)
		}
		want := cdr.Response{
			ID: "CDR-9", ReferenceID: "T001-1", Code: "0",
			Description: "La Guia de remision T001-1 ha sido aceptada", Notes: []string{"Observation"}, XML: cdrXML,
		}
		if !res.Status.Accepted() || !res.CDR.Accepted() || !reflect.DeepEqual(res.CDR, want) {
			t.Errorf("result = %+v, want CDR %+v", res, want)
		}
	})

	t.Run("in process", func(t *testing.T) {
		tr := &greFakeTransport{status: gre.Status{Code: gre.CodeInProcess}}
		res, err := sunat.NewGRE(signer, tr).DespatchStatus(context.Background(), "t")
		if err != nil || !res.Status.InProcess() || !reflect.DeepEqual(res.CDR, cdr.Response{}) {
			t.Errorf("result = %+v, error = %v", res, err)
		}
	})

	t.Run("rejected without CDR", func(t *testing.T) {
		rejection := &gre.Error{Code: "2335", Message: "alterado", HTTPStatus: 200}
		tr := &greFakeTransport{status: gre.Status{Code: gre.CodeRejected, Error: rejection}}
		res, err := sunat.NewGRE(signer, tr).DespatchStatus(context.Background(), "t")
		if err != nil || !res.Status.Rejected() || res.Status.Error != rejection {
			t.Errorf("result = %+v, error = %v", res, err)
		}
	})

	t.Run("rejected with CDR", func(t *testing.T) {
		rz, _ := greCDR(t, g.FileName(), "2335", "rechazada")
		tr := &greFakeTransport{status: gre.Status{Code: gre.CodeRejected, CDRZip: rz}}
		res, err := sunat.NewGRE(signer, tr).DespatchStatus(context.Background(), "t")
		if err != nil || res.CDR.Code != "2335" || res.CDR.Accepted() {
			t.Errorf("result = %+v, error = %v", res, err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		apiErr := &gre.Error{Code: "500", HTTPStatus: 500}
		tr := &greFakeTransport{statusErr: apiErr}
		_, err := sunat.NewGRE(signer, tr).DespatchStatus(context.Background(), "t")
		var got *gre.Error
		if err == nil || !strings.HasPrefix(err.Error(), "sunat: despatch status: ") || !errors.As(err, &got) || got != apiErr {
			t.Errorf("error = %v, want wrapped *gre.Error", err)
		}
	})

	t.Run("invalid CDR keeps the status", func(t *testing.T) {
		tr := &greFakeTransport{status: gre.Status{Code: gre.CodeAccepted, CDRZip: []byte("not a zip")}}
		res, err := sunat.NewGRE(signer, tr).DespatchStatus(context.Background(), "t")
		if err == nil || !strings.HasPrefix(err.Error(), "sunat: parse CDR: ") {
			t.Fatalf("error = %v", err)
		}
		if !res.Status.Accepted() || string(res.Status.CDRZip) != "not a zip" {
			t.Errorf("status lost on parse error: %+v", res.Status)
		}
	})
}

func TestDespatchBuildErrorsStopPipeline(t *testing.T) {
	for _, stage := range []string{"render", "sign"} {
		t.Run(stage, func(t *testing.T) {
			g := greSampleDespatch(catalog.DespatchSender)
			if stage == "render" {
				g.Type = "99"
			}
			signErr := errors.New("signing failed")
			calls := 0
			signer := greSignerFunc(func(doc []byte) ([]byte, error) {
				calls++
				want, err := ubl.Despatch(g)
				if err != nil || !bytes.Equal(doc, want) {
					t.Error("signer must receive ubl.Despatch output")
				}
				return nil, signErr
			})
			tr := &greFakeTransport{}
			client := sunat.NewGRE(signer, tr)
			check := func(err error) {
				t.Helper()
				if err == nil || !strings.HasPrefix(err.Error(), "sunat: ") {
					t.Fatalf("error = %v, want sunat error", err)
				}
				if stage == "sign" && !errors.Is(err, signErr) {
					t.Errorf("error = %v, want wrapped signer error", err)
				}
			}
			xml, err := client.BuildDespatch(g)
			check(err)
			if len(xml) != 0 {
				t.Error("BuildDespatch returned XML after a build failure")
			}
			result, err := client.SendDespatch(context.Background(), g)
			check(err)
			if !reflect.DeepEqual(result, sunat.DespatchResult{}) || tr.sendCalls != 0 {
				t.Error("build failure must return an empty result without calling the transport")
			}
			wantCalls := 2
			if stage == "render" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Errorf("signer calls = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestBuildDespatch(t *testing.T) {
	signer, _ := greSigner(t)
	g := greSampleDespatch(catalog.DespatchSender)
	for name, build := range map[string]func(model.Despatch) ([]byte, error){
		"Client":    sunat.New(signer, nil).BuildDespatch,
		"GREClient": sunat.NewGRE(signer, nil).BuildDespatch,
	} {
		t.Run(name, func(t *testing.T) {
			signed, err := build(g)
			if err != nil {
				t.Fatalf("BuildDespatch() error: %v", err)
			}
			for _, s := range []string{`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Id="` + ubl.SignatureID + `">`, "<cbc:URI>#" + ubl.SignatureID + "</cbc:URI>"} {
				if !bytes.Contains(signed, []byte(s)) {
					t.Errorf("signed XML lacks %s", s)
				}
			}
			if !bytes.Contains(signed, []byte("<cbc:ID>"+g.ID()+"</cbc:ID>")) {
				t.Error("signed XML lost the guide identifier")
			}
		})
	}
	tr := &greFakeTransport{}
	if _, err := sunat.NewGRE(signer, tr).BuildDespatch(g); err != nil || tr.sendCalls+tr.statusCalls != 0 {
		t.Error("BuildDespatch must not call the transport")
	}
}

func TestBuildDespatchVerifiesWithXmlsec1(t *testing.T) {
	if testing.Short() {
		t.Skip("xmlsec1 integration test")
	}
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	for _, kind := range []catalog.DespatchType{catalog.DespatchSender, catalog.DespatchCarrier} {
		t.Run(string(kind), func(t *testing.T) {
			signer, certPEM := greSigner(t)
			signed, err := sunat.NewGRE(signer, nil).BuildDespatch(greSampleDespatch(kind))
			if err != nil {
				t.Fatalf("BuildDespatch() error: %v", err)
			}
			dir := t.TempDir()
			doc, crt := filepath.Join(dir, "signed.xml"), filepath.Join(dir, "cert.pem")
			if err := os.WriteFile(doc, signed, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(crt, certPEM, 0o600); err != nil {
				t.Fatal(err)
			}
			verify := func(file string) ([]byte, error) {
				cmd := exec.Command(bin, "--verify", "--pubkey-cert-pem", crt, file)
				cmd.Env = append(os.Environ(), "OPENSSL_ENABLE_SHA1_SIGNATURES=1")
				return cmd.CombinedOutput()
			}
			if out, err := verify(doc); err != nil {
				t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, out)
			}

			// A change after signing must be detected.
			tampered := filepath.Join(dir, "tampered.xml")
			altered := bytes.Replace(signed, []byte("<cbc:DeliveredQuantity unitCode=\"NIU\">2<"), []byte("<cbc:DeliveredQuantity unitCode=\"NIU\">9<"), 1)
			if bytes.Equal(altered, signed) {
				t.Fatal("tamper target not found in the signed XML")
			}
			if err := os.WriteFile(tampered, altered, 0o600); err != nil {
				t.Fatal(err)
			}
			if out, err := verify(tampered); err == nil {
				t.Fatalf("xmlsec1 accepted a tampered guide\n%s", out)
			}
		})
	}
}
