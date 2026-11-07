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
	"github.com/villano14/sunat-go/internal/testcert"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/sign"
	"github.com/villano14/sunat-go/transport"
	"github.com/villano14/sunat-go/ubl"
)

var batchPET = time.FixedZone("PET", -5*3600)

func batchSampleSummary() model.Summary {
	d := decimal.RequireFromString
	return model.Summary{
		Company:        model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, batchPET),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, batchPET),
		Correlative:    1,
		Currency:       "PEN",
		Lines: []model.SummaryLine{{
			Type: catalog.Receipt, Series: "B001", Number: 1,
			ClientIdentityType: catalog.DNI, ClientNumber: "12345678",
			Status: catalog.SummaryAdd,
			Total:  d("118"), TaxedAmount: d("100"), IGV: d("18"), IGVPercent: d("18"),
		}},
	}
}

func batchSampleVoided() model.Voided {
	return model.Voided{
		Company:        model.Company{RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C."},
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, batchPET),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, batchPET),
		Correlative:    1,
		Lines:          []model.VoidedLine{{Type: catalog.Invoice, Series: "F001", Number: 1, Reason: "ERROR EN DATOS"}},
	}
}

func batchSigner(t *testing.T) (*sign.XMLSigner, []byte) {
	t.Helper()
	key, cert := testcert.New(t)
	return sign.New(sign.Certificate{Key: key, Cert: cert}), testcert.CertPEM(cert)
}

// batchCDR returns a zipped ApplicationResponse with the given response code.
func batchCDR(t *testing.T, ref, code, description string) []byte {
	t.Helper()
	xml := `<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"><cbc:ID>CDR-1</cbc:ID><cac:DocumentResponse><cac:Response><cbc:ReferenceID>` +
		ref + `</cbc:ReferenceID><cbc:ResponseCode>` + code + `</cbc:ResponseCode><cbc:Description>` + description + `</cbc:Description></cac:Response></cac:DocumentResponse></ApplicationResponse>`
	zipped, err := transport.Zip("R-"+ref+".xml", []byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	return zipped
}

// batchFake implements transport.Transport, BatchTransport and ConsultTransport.
type batchFake struct {
	sendCalls, statusCalls, consultCalls int
	ctx                                  context.Context
	fileName                             string
	xml                                  []byte
	ticket                               string
	sendErr                              error

	gotTicket string
	status    transport.Status
	statusErr error

	gotQuery   [4]string
	gotNumber  int64
	consult    transport.Status
	consultErr error
}

func (f *batchFake) SendBill(context.Context, string, []byte) ([]byte, error) {
	return nil, errors.New("unexpected sendBill")
}

func (f *batchFake) SendSummary(ctx context.Context, fileName string, signedXML []byte) (string, error) {
	f.sendCalls++
	f.ctx, f.fileName, f.xml = ctx, fileName, bytes.Clone(signedXML)
	return f.ticket, f.sendErr
}

func (f *batchFake) GetStatus(ctx context.Context, ticket string) (transport.Status, error) {
	f.statusCalls++
	f.ctx, f.gotTicket = ctx, ticket
	return f.status, f.statusErr
}

func (f *batchFake) GetStatusCdr(ctx context.Context, ruc, docType, series string, number int64) (transport.Status, error) {
	f.consultCalls++
	f.ctx = ctx
	f.gotQuery, f.gotNumber = [4]string{ruc, docType, series, ""}, number
	return f.consult, f.consultErr
}

// batchBillOnly implements only transport.Transport.
type batchBillOnly struct{}

func (batchBillOnly) SendBill(context.Context, string, []byte) ([]byte, error) { return nil, nil }

func TestBuildSummaryAndVoided(t *testing.T) {
	signer, _ := batchSigner(t)
	fake := &batchFake{}
	client := sunat.New(signer, fake)

	got, err := client.BuildSummary(batchSampleSummary())
	if err != nil {
		t.Fatalf("BuildSummary() error: %v", err)
	}
	unsigned, err := ubl.Summary(batchSampleSummary())
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := signer.Sign(unsigned); !bytes.Equal(got, want) {
		t.Error("BuildSummary must render ubl.Summary and sign it")
	}

	got, err = client.BuildVoided(batchSampleVoided())
	if err != nil {
		t.Fatalf("BuildVoided() error: %v", err)
	}
	unsigned, err = ubl.Voided(batchSampleVoided())
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := signer.Sign(unsigned); !bytes.Equal(got, want) {
		t.Error("BuildVoided must render ubl.Voided and sign it")
	}
	if fake.sendCalls+fake.statusCalls+fake.consultCalls != 0 {
		t.Error("Build* must not call the transport")
	}
}

func TestBatchBuildErrorsStopPipeline(t *testing.T) {
	signer, _ := batchSigner(t)
	fake := &batchFake{}
	client := sunat.New(signer, fake)
	bad := batchSampleSummary()
	bad.Lines = nil
	badVoided := batchSampleVoided()
	badVoided.Lines = nil

	if xml, err := client.BuildSummary(bad); err == nil || len(xml) != 0 || !strings.HasPrefix(err.Error(), "sunat: build summary: ") {
		t.Errorf("BuildSummary() = %d bytes, %v", len(xml), err)
	}
	if xml, err := client.BuildVoided(badVoided); err == nil || len(xml) != 0 || !strings.HasPrefix(err.Error(), "sunat: build voided: ") {
		t.Errorf("BuildVoided() = %d bytes, %v", len(xml), err)
	}
	if res, err := client.SendSummary(context.Background(), bad); err == nil || !reflect.DeepEqual(res, sunat.BatchResult{}) {
		t.Errorf("SendSummary() = %+v, %v", res, err)
	}
	if res, err := client.SendVoided(context.Background(), badVoided); err == nil || !reflect.DeepEqual(res, sunat.BatchResult{}) {
		t.Errorf("SendVoided() = %+v, %v", res, err)
	}

	signErr := errors.New("signing failed")
	failing := sunat.New(batchSignerFunc(func([]byte) ([]byte, error) { return nil, signErr }), fake)
	if _, err := failing.SendSummary(context.Background(), batchSampleSummary()); !errors.Is(err, signErr) {
		t.Errorf("SendSummary() error = %v, want wrapped signer error", err)
	}
	if fake.sendCalls != 0 {
		t.Error("build failures must not reach the transport")
	}
}

func TestSendSummaryAndVoided(t *testing.T) {
	signer, _ := batchSigner(t)
	for _, tt := range []struct {
		name, fileName string
		send           func(*sunat.Client, context.Context) (sunat.BatchResult, error)
		root           string
	}{
		{"summary", "20000000001-RC-20261028-1", func(c *sunat.Client, ctx context.Context) (sunat.BatchResult, error) {
			return c.SendSummary(ctx, batchSampleSummary())
		}, "<SummaryDocuments "},
		{"voided", "20000000001-RA-20261028-1", func(c *sunat.Client, ctx context.Context) (sunat.BatchResult, error) {
			return c.SendVoided(ctx, batchSampleVoided())
		}, "<VoidedDocuments "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := &batchFake{ticket: "202600012345"}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result, err := tt.send(sunat.New(signer, fake), ctx)
			if err != nil {
				t.Fatalf("send error: %v", err)
			}
			if result.Ticket != "202600012345" {
				t.Errorf("Ticket = %q", result.Ticket)
			}
			if fake.sendCalls != 1 || fake.fileName != tt.fileName || fake.ctx != ctx {
				t.Errorf("calls=%d fileName=%q context passed through=%v", fake.sendCalls, fake.fileName, fake.ctx == ctx)
			}
			if !bytes.Equal(result.XML, fake.xml) || !bytes.Contains(result.XML, []byte("<ds:Signature ")) || !bytes.Contains(result.XML, []byte(tt.root)) {
				t.Error("BatchResult.XML must be the signed XML sent to the transport")
			}
		})
	}
}

func TestSendBatchErrorsKeepXML(t *testing.T) {
	signer, _ := batchSigner(t)
	fault := &transport.Error{Code: "0151", Message: "Invalid ZIP file name", FaultCode: "soap-env:Client.0151"}
	for _, tt := range []struct {
		name   string
		fake   *batchFake
		client func(*batchFake) *sunat.Client
		want   error
	}{
		{"SOAP fault", &batchFake{sendErr: fault}, func(f *batchFake) *sunat.Client { return sunat.New(signer, f) }, fault},
		{"HTTP error", &batchFake{sendErr: &transport.HTTPError{StatusCode: 502}}, func(f *batchFake) *sunat.Client { return sunat.New(signer, f) }, nil},
		{"empty ticket", &batchFake{}, func(f *batchFake) *sunat.Client { return sunat.New(signer, f) }, nil},
		{"unsupported", nil, func(*batchFake) *sunat.Client { return sunat.New(signer, batchBillOnly{}) }, sunat.ErrBatchUnsupported},
		{"nil transport", nil, func(*batchFake) *sunat.Client { return sunat.New(signer, nil) }, sunat.ErrBatchUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client(tt.fake)
			for kind, send := range map[string]func() (sunat.BatchResult, error){
				"summary": func() (sunat.BatchResult, error) {
					return client.SendSummary(context.Background(), batchSampleSummary())
				},
				"voided": func() (sunat.BatchResult, error) {
					return client.SendVoided(context.Background(), batchSampleVoided())
				},
			} {
				result, err := send()
				if err == nil || !strings.HasPrefix(err.Error(), "sunat: send "+kind+": ") {
					t.Fatalf("%s: error = %v, want send error", kind, err)
				}
				if tt.want != nil && !errors.Is(err, tt.want) {
					t.Errorf("%s: error = %v, want wrapped %v", kind, err, tt.want)
				}
				if tt.want == fault {
					var got *transport.Error
					if !errors.As(err, &got) || got != fault {
						t.Errorf("%s: error = %v, want original *transport.Error", kind, err)
					}
				}
				if len(result.XML) == 0 || !bytes.Contains(result.XML, []byte("<ds:Signature ")) || result.Ticket != "" {
					t.Errorf("%s: send errors must keep the signed XML and no ticket: %+v", kind, result)
				}
			}
		})
	}
}

func TestStatusResults(t *testing.T) {
	signer, _ := batchSigner(t)
	accepted := batchCDR(t, "RC-20261028-1", "0", "El Resumen diario RC-20261028-1 ha sido aceptado")
	observed := batchCDR(t, "RC-20261028-1", "4000", "Aceptado con observaciones")
	rejected := batchCDR(t, "RC-20261028-1", "2223", "El comprobante fue enviado anteriormente")
	for _, tt := range []struct {
		name        string
		status      transport.Status
		state       sunat.BatchState
		code, descr string
		hasCDR      bool
	}{
		{"in process", transport.Status{Code: "98"}, sunat.BatchInProcess, "98", "ticket still in process", false},
		{"accepted", transport.Status{Code: "0", Content: accepted}, sunat.BatchAccepted, "0", "El Resumen diario RC-20261028-1 ha sido aceptado", true},
		{"accepted with observations", transport.Status{Code: "0", Content: observed}, sunat.BatchAccepted, "4000", "Aceptado con observaciones", true},
		{"processed with errors", transport.Status{Code: "99", Content: rejected}, sunat.BatchRejected, "2223", "El comprobante fue enviado anteriormente", true},
		{"rejecting CDR on code 0", transport.Status{Code: "0", Content: rejected}, sunat.BatchRejected, "2223", "El comprobante fue enviado anteriormente", true},
		{"errors without CDR", transport.Status{Code: "99"}, sunat.BatchRejected, "99", "processed with errors; SUNAT returned no CDR", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := &batchFake{status: tt.status}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			res, err := sunat.New(signer, fake).Status(ctx, "202600012345")
			if err != nil {
				t.Fatalf("Status() error: %v", err)
			}
			if fake.statusCalls != 1 || fake.gotTicket != "202600012345" || fake.ctx != ctx {
				t.Errorf("transport calls=%d ticket=%q context passed through=%v", fake.statusCalls, fake.gotTicket, fake.ctx == ctx)
			}
			if res.State != tt.state || res.StatusCode != tt.status.Code || res.Code != tt.code || res.Description != tt.descr {
				t.Errorf("result = %+v", res)
			}
			if res.InProcess() != (tt.state == sunat.BatchInProcess) || res.Accepted() != (tt.state == sunat.BatchAccepted) || res.Rejected() != (tt.state == sunat.BatchRejected) {
				t.Errorf("predicates disagree with state %q", res.State)
			}
			if tt.hasCDR {
				if !bytes.Equal(res.CDRZip, tt.status.Content) || res.CDR.Code != tt.code || len(res.CDR.XML) == 0 || res.CDR.ReferenceID != "RC-20261028-1" {
					t.Errorf("CDR = %+v", res.CDR)
				}
			} else if !reflect.DeepEqual(res.CDR, cdr.Response{}) || len(res.CDRZip) != 0 {
				t.Errorf("unexpected CDR: %+v", res.CDR)
			}
		})
	}
}

func TestStatusErrors(t *testing.T) {
	signer, _ := batchSigner(t)
	fault := &transport.Error{Code: "0127", Message: "El ticket no existe", FaultCode: "soap-env:Client.0127"}
	for _, tt := range []struct {
		name   string
		client *sunat.Client
		prefix string
		want   error
	}{
		{"SOAP fault", sunat.New(signer, &batchFake{statusErr: fault}), "sunat: status: ", fault},
		{"canceled", sunat.New(signer, &batchFake{statusErr: context.Canceled}), "sunat: status: ", context.Canceled},
		{"unexpected code", sunat.New(signer, &batchFake{status: transport.Status{Code: "7"}}), "sunat: status: ", nil},
		{"success without CDR", sunat.New(signer, &batchFake{status: transport.Status{Code: "0"}}), "sunat: status: ", nil},
		{"invalid CDR", sunat.New(signer, &batchFake{status: transport.Status{Code: "0", Content: []byte("not a zip")}}), "sunat: parse CDR: ", nil},
		{"unsupported", sunat.New(signer, batchBillOnly{}), "sunat: status: ", sunat.ErrBatchUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.client.Status(context.Background(), "1")
			if err == nil || !strings.HasPrefix(err.Error(), tt.prefix) {
				t.Fatalf("error = %v, want prefix %q", err, tt.prefix)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want wrapped %v", err, tt.want)
			}
			if tt.want == fault {
				var got *transport.Error
				if !errors.As(err, &got) || got != fault {
					t.Errorf("error = %v, want original *transport.Error", err)
				}
			}
			if res.Accepted() || res.InProcess() {
				t.Errorf("an error must not report %q", res.State)
			}
		})
	}
}

func TestQueryCDR(t *testing.T) {
	signer, _ := batchSigner(t)
	accepted := batchCDR(t, "F001-12", "0", "La Factura numero F001-12, ha sido aceptada")
	fake := &batchFake{consult: transport.Status{Code: "0004", Message: "El comprobante existe", Content: accepted}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	res, err := sunat.New(signer, fake).QueryCDR(ctx, "20000000001", catalog.Invoice, "F001", 12)
	if err != nil {
		t.Fatalf("QueryCDR() error: %v", err)
	}
	if fake.consultCalls != 1 || fake.ctx != ctx || fake.gotQuery != [4]string{"20000000001", "01", "F001", ""} || fake.gotNumber != 12 {
		t.Errorf("transport got %v %d (calls=%d)", fake.gotQuery, fake.gotNumber, fake.consultCalls)
	}
	if !res.Found() || !res.Accepted() || res.StatusCode != "0004" || res.Message != "El comprobante existe" ||
		res.CDR.Code != "0" || res.CDR.ReferenceID != "F001-12" || !bytes.Equal(res.CDRZip, accepted) {
		t.Errorf("result = %+v", res)
	}
}

func TestQueryCDRNotFound(t *testing.T) {
	signer, _ := batchSigner(t)
	fake := &batchFake{consult: transport.Status{Code: "0011", Message: "El comprobante de pago electrónico no existe"}}
	res, err := sunat.New(signer, fake).QueryCDR(context.Background(), "20000000001", catalog.Invoice, "F001", 99)
	if err != nil {
		t.Fatalf("QueryCDR() error: %v", err)
	}
	if res.Found() || res.Accepted() || res.StatusCode != "0011" || !strings.Contains(res.Message, "no existe") {
		t.Errorf("result = %+v", res)
	}
}

func TestQueryCDRRejectedCDR(t *testing.T) {
	signer, _ := batchSigner(t)
	fake := &batchFake{consult: transport.Status{Code: "0004", Content: batchCDR(t, "F001-1", "2335", "rechazado")}}
	res, err := sunat.New(signer, fake).QueryCDR(context.Background(), "20000000001", catalog.Invoice, "F001", 1)
	if err != nil {
		t.Fatalf("QueryCDR() error: %v", err)
	}
	if !res.Found() || res.Accepted() || res.CDR.Code != "2335" {
		t.Errorf("result = %+v", res)
	}
}

func TestQueryCDRErrors(t *testing.T) {
	signer, _ := batchSigner(t)
	fault := &transport.Error{Code: "0125", Message: "No se pudo obtener la constancia"}
	for _, tt := range []struct {
		name   string
		client *sunat.Client
		prefix string
		want   error
	}{
		{"SOAP fault", sunat.New(signer, &batchFake{consultErr: fault}), "sunat: query CDR: ", fault},
		{"HTTP error", sunat.New(signer, &batchFake{consultErr: &transport.HTTPError{StatusCode: 503}}), "sunat: query CDR: ", nil},
		{"no beta endpoint", sunat.New(signer, &batchFake{consultErr: transport.ErrNoConsultEndpoint}), "sunat: query CDR: ", transport.ErrNoConsultEndpoint},
		{"invalid CDR", sunat.New(signer, &batchFake{consult: transport.Status{Code: "0004", Content: []byte("not a zip")}}), "sunat: parse CDR: ", nil},
		{"unsupported", sunat.New(signer, batchBillOnly{}), "sunat: query CDR: ", sunat.ErrConsultUnsupported},
		{"nil transport", sunat.New(signer, nil), "sunat: query CDR: ", sunat.ErrConsultUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.QueryCDR(context.Background(), "20000000001", catalog.Invoice, "F001", 1)
			if err == nil || !strings.HasPrefix(err.Error(), tt.prefix) {
				t.Fatalf("error = %v, want prefix %q", err, tt.prefix)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want wrapped %v", err, tt.want)
			}
		})
	}
}

// TestBatchSOAPClientImplementsInterfaces proves the real client is usable by
// the facade, so the type assertions never silently fall back to unsupported.
func TestBatchSOAPClientImplementsInterfaces(t *testing.T) {
	var tr transport.Transport = transport.New(transport.BillBeta, transport.Credentials{})
	if _, ok := tr.(transport.BatchTransport); !ok {
		t.Error("SOAPClient must implement transport.BatchTransport")
	}
	if _, ok := tr.(transport.ConsultTransport); !ok {
		t.Error("SOAPClient must implement transport.ConsultTransport")
	}
}

func TestBuildBatchVerifiesWithXmlsec1(t *testing.T) {
	if testing.Short() {
		t.Skip("xmlsec1 integration test")
	}
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	signer, certPEM := batchSigner(t)
	client := sunat.New(signer, nil)
	summary, err := client.BuildSummary(batchSampleSummary())
	if err != nil {
		t.Fatalf("BuildSummary() error: %v", err)
	}
	voided, err := client.BuildVoided(batchSampleVoided())
	if err != nil {
		t.Fatalf("BuildVoided() error: %v", err)
	}
	dir := t.TempDir()
	crt := filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(crt, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, signed := range map[string][]byte{"summary": summary, "voided": voided} {
		t.Run(name, func(t *testing.T) {
			doc := filepath.Join(dir, name+".xml")
			if err := os.WriteFile(doc, signed, 0o600); err != nil {
				t.Fatal(err)
			}
			verify := func(doc string) ([]byte, error) {
				cmd := exec.Command(bin, "--verify", "--pubkey-cert-pem", crt, doc)
				cmd.Env = append(os.Environ(), "OPENSSL_ENABLE_SHA1_SIGNATURES=1")
				return cmd.CombinedOutput()
			}
			if out, err := verify(doc); err != nil {
				t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, out)
			}
			// A tampered ID must break the signature.
			tampered := filepath.Join(dir, name+"-tampered.xml")
			if err := os.WriteFile(tampered, bytes.Replace(signed, []byte("20261028"), []byte("20261029"), 1), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := verify(tampered); err == nil {
				t.Error("xmlsec1 accepted a tampered document")
			}
		})
	}
}

type batchSignerFunc func([]byte) ([]byte, error)

func (f batchSignerFunc) Sign(doc []byte) ([]byte, error) { return f(doc) }
