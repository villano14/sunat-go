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

func sampleRetention() model.Retention {
	d := decimal.RequireFromString
	day := time.Date(2026, 10, 26, 0, 0, 0, 0, time.FixedZone("PET", -5*3600))
	return model.Retention{
		Series: "R001", Number: 1,
		IssueDate: time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Company: model.Company{
			RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C.",
			Address: model.Address{
				Ubigeo: "150101", Department: "LIMA", Province: "LIMA",
				District: "LIMA", Line: "AV. LOS OLIVOS 123",
			},
		},
		Supplier: model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "PROVEEDOR S.A.C."},
		Regime:   catalog.RetentionRate3, Rate: d("3"),
		TotalRetained: d("3"), TotalPaid: d("97"),
		Details: []model.RetentionDetail{{
			DocumentType: catalog.Invoice, Number: "F001-1", IssueDate: day,
			Total: d("100"), Currency: "PEN",
			Payments:      []model.Payment{{Currency: "PEN", Amount: d("100"), Date: day}},
			RetentionDate: day, Retained: d("3"), NetPayable: d("97"),
		}},
	}
}

func samplePerception() model.Perception {
	d := decimal.RequireFromString
	day := time.Date(2026, 10, 26, 0, 0, 0, 0, time.FixedZone("PET", -5*3600))
	return model.Perception{
		Series: "P001", Number: 1,
		IssueDate: time.Date(2026, 10, 27, 13, 5, 0, 0, time.FixedZone("PET", -5*3600)),
		Company: model.Company{
			RUC: "20000000001", LegalName: "EMPRESA DE PRUEBA S.A.C.",
			Address: model.Address{
				Ubigeo: "150101", Department: "LIMA", Province: "LIMA",
				District: "LIMA", Line: "AV. LOS OLIVOS 123",
			},
		},
		Customer: model.Client{IdentityType: catalog.RUC, Number: "20123456789", Name: "CLIENTE S.A.C."},
		Regime:   catalog.PerceptionInternalSale, Rate: d("2"),
		TotalPerceived: d("2"), TotalCollected: d("102"),
		Details: []model.PerceptionDetail{{
			DocumentType: catalog.Invoice, Number: "F001-1", IssueDate: day,
			Total: d("100"), Currency: "PEN",
			Collections:    []model.Payment{{Currency: "PEN", Amount: d("102"), Date: day}},
			PerceptionDate: day, Perceived: d("2"), NetCollectable: d("102"),
		}},
	}
}

func withholdingSigner(t *testing.T) (*sign.XMLSigner, []byte) {
	t.Helper()
	key, cert := testcert.New(t)
	return sign.New(sign.Certificate{Key: key, Cert: cert}), testcert.CertPEM(cert)
}

type withholdingTransport struct {
	calls    int
	ctx      context.Context
	fileName string
	xml      []byte
	response []byte
	err      error
}

func (f *withholdingTransport) SendBill(ctx context.Context, fileName string, signedXML []byte) ([]byte, error) {
	f.calls++
	f.ctx, f.fileName, f.xml = ctx, fileName, bytes.Clone(signedXML)
	return f.response, f.err
}

type withholdingSignerFunc func([]byte) ([]byte, error)

func (f withholdingSignerFunc) Sign(doc []byte) ([]byte, error) { return f(doc) }

// withholdingCase abstracts over retention and perception so every facade
// behavior is asserted for both documents.
type withholdingCase struct {
	name     string
	kind     string // error message noun
	id       string
	fileName string
	render   func() ([]byte, error)
	build    func(*sunat.Client) ([]byte, error)
	send     func(context.Context, *sunat.Client) (sunat.Result, error)
	breakIt  func() // makes rendering fail
	restore  func()
	wantFile string
	root     string
}

func withholdingCases() []withholdingCase {
	ret, per := sampleRetention(), samplePerception()
	return []withholdingCase{
		{
			name: "retention", kind: "retention", id: ret.ID(), fileName: ret.FileName(), wantFile: "20000000001-20-R001-1", root: "<Retention ",
			render: func() ([]byte, error) { return ubl.Retention(ret) },
			build:  func(c *sunat.Client) ([]byte, error) { return c.BuildRetention(ret) },
			send: func(ctx context.Context, c *sunat.Client) (sunat.Result, error) {
				return c.SendRetention(ctx, ret)
			},
			breakIt: func() { ret.Regime = "99" },
			restore: func() { ret = sampleRetention() },
		},
		{
			name: "perception", kind: "perception", id: per.ID(), fileName: per.FileName(), wantFile: "20000000001-40-P001-1", root: "<Perception ",
			render: func() ([]byte, error) { return ubl.Perception(per) },
			build:  func(c *sunat.Client) ([]byte, error) { return c.BuildPerception(per) },
			send: func(ctx context.Context, c *sunat.Client) (sunat.Result, error) {
				return c.SendPerception(ctx, per)
			},
			breakIt: func() { per.Regime = "99" },
			restore: func() { per = samplePerception() },
		},
	}
}

func withholdingCDR(t *testing.T, fileName, id string) ([]byte, []byte) {
	t.Helper()
	cdrXML := []byte(`<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"><cbc:ID>CDR-1</cbc:ID><cbc:Note>Observation</cbc:Note><cac:DocumentResponse><cac:Response><cbc:ReferenceID>` + id + `</cbc:ReferenceID><cbc:ResponseCode>0</cbc:ResponseCode><cbc:Description>Accepted</cbc:Description></cac:Response></cac:DocumentResponse></ApplicationResponse>`)
	zipped, err := transport.Zip("R-"+fileName+".xml", cdrXML)
	if err != nil {
		t.Fatal(err)
	}
	return zipped, cdrXML
}

func TestSendWithholding(t *testing.T) {
	for _, tc := range withholdingCases() {
		t.Run(tc.name, func(t *testing.T) {
			signer, _ := withholdingSigner(t)
			zipped, cdrXML := withholdingCDR(t, tc.fileName, tc.id)
			tr := &withholdingTransport{response: zipped}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result, err := tc.send(ctx, sunat.New(signer, tr))
			if err != nil {
				t.Fatalf("Send error: %v", err)
			}
			if tr.calls != 1 || tr.fileName != tc.wantFile || tc.fileName != tc.wantFile || tr.ctx != ctx {
				t.Errorf("transport calls=%d fileName=%q (want %q) context passed through=%v", tr.calls, tr.fileName, tc.wantFile, tr.ctx == ctx)
			}
			if !bytes.Equal(result.XML, tr.xml) || !bytes.Contains(result.XML, []byte("<ds:Signature ")) ||
				!bytes.Contains(result.XML, []byte(tc.root)) {
				t.Error("Result.XML must be the signed document sent to the transport")
			}
			want := cdr.Response{
				ID: "CDR-1", ReferenceID: tc.id, Code: "0", Description: "Accepted",
				Notes: []string{"Observation"}, XML: cdrXML,
			}
			if !result.CDR.Accepted() || !reflect.DeepEqual(result.CDR, want) {
				t.Errorf("CDR = %+v, want %+v", result.CDR, want)
			}
		})
	}
}

func TestSendWithholdingErrorsKeepXML(t *testing.T) {
	signer, _ := withholdingSigner(t)
	fault := &transport.Error{Code: "0151", Message: "Invalid ZIP file name", FaultCode: "soap-env:Client.0151"}
	for _, tc := range withholdingCases() {
		for _, tt := range []struct {
			name     string
			response []byte
			err      error
			prefix   string
		}{
			{name: "SOAP fault", err: fault, prefix: "sunat: send " + tc.kind + ": "},
			{name: "canceled context", err: context.Canceled, prefix: "sunat: send " + tc.kind + ": "},
			{name: "invalid CDR", response: []byte("not a zip"), prefix: "sunat: parse CDR: "},
		} {
			t.Run(tc.name+"/"+tt.name, func(t *testing.T) {
				tr := &withholdingTransport{response: tt.response, err: tt.err}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tt.err == context.Canceled {
					cancel()
				}
				result, err := tc.send(ctx, sunat.New(signer, tr))
				if err == nil || !strings.HasPrefix(err.Error(), tt.prefix) {
					t.Fatalf("error = %v, want prefix %q", err, tt.prefix)
				}
				if tt.err != nil && !errors.Is(err, tt.err) {
					t.Errorf("error = %v, want wrapped %v", err, tt.err)
				}
				if tt.err == fault {
					var got *transport.Error
					if !errors.As(err, &got) || got != fault || got.Code != "0151" {
						t.Errorf("error = %v, want original *transport.Error with code 0151", err)
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
}

func TestWithholdingBuildErrorsStopPipeline(t *testing.T) {
	for _, tc := range withholdingCases() {
		for _, stage := range []string{"render", "sign"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				signErr := errors.New("signing failed")
				calls := 0
				signer := withholdingSignerFunc(func(doc []byte) ([]byte, error) {
					calls++
					want, err := tc.render()
					if err != nil || !bytes.Equal(doc, want) {
						t.Error("signer must receive the ubl output")
					}
					return nil, signErr
				})
				if stage == "render" {
					tc.breakIt()
					defer tc.restore()
				}
				tr := &withholdingTransport{}
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
				xml, err := tc.build(client)
				checkError(err)
				if len(xml) != 0 {
					t.Error("Build returned XML after a failure")
				}
				result, err := tc.send(context.Background(), client)
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
}

func TestBuildWithholding(t *testing.T) {
	for _, tc := range withholdingCases() {
		t.Run(tc.name, func(t *testing.T) {
			signer, _ := withholdingSigner(t)
			tr := &withholdingTransport{}
			xml, err := tc.build(sunat.New(signer, tr))
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}
			unsigned, err := tc.render()
			if err != nil {
				t.Fatal(err)
			}
			want, err := signer.Sign(unsigned)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(xml, want) || tr.calls != 0 {
				t.Error("Build must only render and sign, without calling transport")
			}
		})
	}
}

// TestBuildWithholdingVerifiesWithXmlsec1 proves the signer handles the
// SUNAT-specific Retention-1/Perception-1 roots, not only UBL Invoice.
func TestBuildWithholdingVerifiesWithXmlsec1(t *testing.T) {
	if testing.Short() {
		t.Skip("xmlsec1 integration test")
	}
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	for _, tc := range withholdingCases() {
		t.Run(tc.name, func(t *testing.T) {
			signer, certPEM := withholdingSigner(t)
			xml, err := tc.build(sunat.New(signer, nil))
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}
			dir := t.TempDir()
			doc, crt := filepath.Join(dir, "signed.xml"), filepath.Join(dir, "cert.pem")
			if err := os.WriteFile(doc, xml, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(crt, certPEM, 0o600); err != nil {
				t.Fatal(err)
			}
			verify := func(path string) ([]byte, error) {
				cmd := exec.Command(bin, "--verify", "--pubkey-cert-pem", crt, path)
				cmd.Env = append(os.Environ(), "OPENSSL_ENABLE_SHA1_SIGNATURES=1")
				return cmd.CombinedOutput()
			}
			if out, err := verify(doc); err != nil {
				t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, out)
			}

			tampered := filepath.Join(dir, "tampered.xml")
			changed := bytes.Replace(xml, []byte(">100.00<"), []byte(">1.00<"), 1)
			if bytes.Equal(changed, xml) {
				t.Fatal("tamper target not found in signed XML")
			}
			if err := os.WriteFile(tampered, changed, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := verify(tampered); err == nil {
				t.Error("xmlsec1 accepted a tampered document")
			}
		})
	}
}
