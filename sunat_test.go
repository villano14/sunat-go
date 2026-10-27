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

func sampleInvoice() model.Invoice {
	d := decimal.RequireFromString
	return model.Invoice{
		Type: catalog.Invoice, Series: "F001", Number: 1,
		IssueDate: time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Currency:  "PEN",
		Company: model.Company{
			RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C.",
			Address: model.Address{
				Ubigeo: "150101", Department: "LIMA", Province: "LIMA",
				District: "LIMA", Line: "AV. LOS OLIVOS 123",
			},
		},
		Client:      model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE S.A.C."},
		TaxedAmount: d("200"), IGV: d("36"), TotalTaxes: d("36"),
		LineExtension: d("200"), TaxInclusive: d("236"), Payable: d("236"),
		Lines: []model.InvoiceLine{{
			ProductCode: "P001", Unit: "NIU", Description: "PRODUCTO UNO",
			Quantity: d("2"), UnitValue: d("100"), UnitPrice: d("118"),
			Affectation: catalog.TaxedOnerous, TaxBase: d("200"), IGVPercent: d("18"),
			IGV: d("36"), TotalTaxes: d("36"), LineAmount: d("200"),
		}},
		Legends: []model.Legend{{Code: catalog.AmountInWords, Value: "SON DOSCIENTOS TREINTA Y SEIS CON 00/100 SOLES"}},
	}
}

func newSigner(t *testing.T) (*sign.XMLSigner, []byte) {
	t.Helper()
	key, cert := testcert.New(t)
	return sign.New(sign.Certificate{Key: key, Cert: cert}), testcert.CertPEM(cert)
}

type fakeTransport struct {
	calls    int
	ctx      context.Context
	fileName string
	xml      []byte
	response []byte
	err      error
}

func (f *fakeTransport) SendBill(ctx context.Context, fileName string, signedXML []byte) ([]byte, error) {
	f.calls++
	f.ctx, f.fileName, f.xml = ctx, fileName, bytes.Clone(signedXML)
	return f.response, f.err
}

type signerFunc func([]byte) ([]byte, error)

func (f signerFunc) Sign(doc []byte) ([]byte, error) { return f(doc) }

func TestSendInvoice(t *testing.T) {
	inv := sampleInvoice()
	signer, _ := newSigner(t)
	cdrXML := []byte(`<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"><cbc:ID>CDR-1</cbc:ID><cbc:Note>Observation</cbc:Note><cac:DocumentResponse><cac:Response><cbc:ReferenceID>F001-1</cbc:ReferenceID><cbc:ResponseCode>0</cbc:ResponseCode><cbc:Description>Accepted</cbc:Description></cac:Response></cac:DocumentResponse></ApplicationResponse>`)
	zipped, err := transport.Zip("R-"+inv.FileName()+".xml", cdrXML)
	if err != nil {
		t.Fatal(err)
	}
	tr := &fakeTransport{response: zipped}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := sunat.New(signer, tr).SendInvoice(ctx, inv)
	if err != nil {
		t.Fatalf("SendInvoice() error: %v", err)
	}
	if tr.calls != 1 || tr.fileName != inv.FileName() || tr.ctx != ctx {
		t.Errorf("transport calls=%d fileName=%q context passed through=%v", tr.calls, tr.fileName, tr.ctx == ctx)
	}
	if len(result.XML) == 0 || !bytes.Equal(result.XML, tr.xml) || !bytes.Contains(result.XML, []byte("<ds:Signature ")) {
		t.Error("Result.XML must be the signed XML sent to the transport")
	}
	want := cdr.Response{
		ID: "CDR-1", ReferenceID: inv.ID(), Code: "0", Description: "Accepted",
		Notes: []string{"Observation"}, XML: cdrXML,
	}
	if !result.CDR.Accepted() || !reflect.DeepEqual(result.CDR, want) {
		t.Errorf("CDR = %+v, want %+v", result.CDR, want)
	}
}

func TestSendInvoiceErrorsKeepXML(t *testing.T) {
	signer, _ := newSigner(t)
	fault := &transport.Error{Code: "0151", Message: "Invalid ZIP file name", FaultCode: "soap-env:Client.0151"}
	for _, tt := range []struct {
		name     string
		response []byte
		err      error
		prefix   string
	}{
		{name: "SOAP fault", err: fault, prefix: "sunat: send invoice: "},
		{name: "canceled context", err: context.Canceled, prefix: "sunat: send invoice: "},
		{name: "invalid CDR", response: []byte("not a zip"), prefix: "sunat: parse CDR: "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := &fakeTransport{response: tt.response, err: tt.err}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.err == context.Canceled {
				cancel()
			}
			result, err := sunat.New(signer, tr).SendInvoice(ctx, sampleInvoice())
			if err == nil || !strings.HasPrefix(err.Error(), tt.prefix) {
				t.Fatalf("error = %v, want prefix %q", err, tt.prefix)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("error = %v, want wrapped %v", err, tt.err)
			}
			if tt.err == fault {
				var got *transport.Error
				if !errors.As(err, &got) || got != fault {
					t.Errorf("error = %v, want original *transport.Error", err)
				}
			}
			if tr.calls != 1 || tr.ctx != ctx || len(result.XML) == 0 || !bytes.Equal(result.XML, tr.xml) {
				t.Error("send/parse errors must preserve the signed XML and pass context through")
			}
			if !reflect.DeepEqual(result.CDR, cdr.Response{}) {
				t.Errorf("CDR = %+v, want empty response", result.CDR)
			}
		})
	}
}

func TestInvoiceBuildErrorsStopPipeline(t *testing.T) {
	for _, stage := range []string{"render", "sign"} {
		t.Run(stage, func(t *testing.T) {
			inv := sampleInvoice()
			if stage == "render" {
				inv.Lines[0].Affectation = "99"
			}
			signErr := errors.New("signing failed")
			calls := 0
			signer := signerFunc(func(doc []byte) ([]byte, error) {
				calls++
				want, err := ubl.Invoice(inv)
				if err != nil || !bytes.Equal(doc, want) {
					t.Error("signer must receive ubl.Invoice output")
				}
				return nil, signErr
			})
			tr := &fakeTransport{}
			client := sunat.New(signer, tr)
			checkError := func(err error) {
				t.Helper()
				if err == nil || !strings.HasPrefix(err.Error(), "sunat: ") {
					t.Fatalf("error = %v, want sunat error", err)
				}
				if stage == "sign" && !errors.Is(err, signErr) {
					t.Errorf("error = %v, want wrapped signer error", err)
				}
			}
			xml, err := client.BuildInvoice(inv)
			checkError(err)
			if len(xml) != 0 {
				t.Error("BuildInvoice returned XML after a build failure")
			}
			result, err := client.SendInvoice(context.Background(), inv)
			checkError(err)
			if !reflect.DeepEqual(result, sunat.Result{}) || tr.calls != 0 {
				t.Error("build failure must return an empty result without calling transport")
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

func TestBuildInvoice(t *testing.T) {
	signer, _ := newSigner(t)
	tr := &fakeTransport{}
	inv := sampleInvoice()
	xml, err := sunat.New(signer, tr).BuildInvoice(inv)
	if err != nil {
		t.Fatalf("BuildInvoice() error: %v", err)
	}
	unsigned, err := ubl.Invoice(inv)
	if err != nil {
		t.Fatal(err)
	}
	want, err := signer.Sign(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(xml, want) || tr.calls != 0 {
		t.Error("BuildInvoice must only render and sign, without calling transport")
	}
}

func TestBuildInvoiceVerifiesWithXmlsec1(t *testing.T) {
	if testing.Short() {
		t.Skip("xmlsec1 integration test")
	}
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	signer, certPEM := newSigner(t)
	// The real UBL root already declares xmlns:ds; verify it independently.
	xml, err := sunat.New(signer, nil).BuildInvoice(sampleInvoice())
	if err != nil {
		t.Fatalf("BuildInvoice() error: %v", err)
	}
	dir := t.TempDir()
	doc, crt := filepath.Join(dir, "signed.xml"), filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(doc, xml, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crt, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--verify", "--pubkey-cert-pem", crt, doc)
	cmd.Env = append(os.Environ(), "OPENSSL_ENABLE_SHA1_SIGNATURES=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, out)
	}
}
