package sunat_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/transport"
	"github.com/villano14/sunat-go/ubl"
)

func sampleNote(kind catalog.DocumentType) model.Note {
	inv := sampleInvoice()
	return model.Note{
		Type: kind, Series: "FC01", Number: 3, IssueDate: inv.IssueDate, Currency: inv.Currency,
		AffectedType: catalog.Invoice, AffectedNumber: "F001-45", Reason: "01", Description: "AJUSTE & <MOTIVO>",
		Company: inv.Company, Client: inv.Client, TaxedAmount: inv.TaxedAmount,
		IGV: inv.IGV, TotalTaxes: inv.TotalTaxes, Payable: inv.Payable,
		Lines: inv.Lines, Legends: inv.Legends,
	}
}

var noteTypes = []catalog.DocumentType{catalog.CreditNote, catalog.DebitNote}

func noteCDR(t *testing.T, n model.Note, code string) ([]byte, []byte) {
	t.Helper()
	raw := []byte(`<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"><cbc:ID>CDR-1</cbc:ID><cbc:Note>Observation</cbc:Note><cac:DocumentResponse><cac:Response><cbc:ReferenceID>` + n.ID() + `</cbc:ReferenceID><cbc:ResponseCode>` + code + `</cbc:ResponseCode><cbc:Description>Outcome</cbc:Description></cac:Response></cac:DocumentResponse></ApplicationResponse>`)
	zipped, err := transport.Zip("R-"+n.FileName()+".xml", raw)
	if err != nil {
		t.Fatal(err)
	}
	return zipped, raw
}

func TestBuildNote(t *testing.T) {
	signer, _ := newSigner(t)
	for _, kind := range noteTypes {
		t.Run(string(kind), func(t *testing.T) {
			n := sampleNote(kind)
			tr := &fakeTransport{}
			got, err := sunat.New(signer, tr).BuildNote(n)
			if err != nil {
				t.Fatal(err)
			}
			unsigned, err := ubl.Note(n)
			if err != nil {
				t.Fatal(err)
			}
			want, err := signer.Sign(unsigned)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) || tr.calls != 0 {
				t.Error("BuildNote must only render and sign, without calling transport")
			}
			var doc struct {
				Signature struct {
					ID string `xml:"Id,attr"`
				} `xml:"UBLExtensions>UBLExtension>ExtensionContent>Signature"`
				URI string `xml:"Signature>DigitalSignatureAttachment>ExternalReference>URI"`
			}
			if err := xml.Unmarshal(got, &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Signature.ID != ubl.SignatureID || doc.URI != "#"+doc.Signature.ID {
				t.Errorf("signature ID = %q, reference = %q", doc.Signature.ID, doc.URI)
			}
		})
	}
}

func TestBuildNoteErrorsStopPipeline(t *testing.T) {
	for _, kind := range noteTypes {
		for _, stage := range []string{"render", "sign"} {
			t.Run(string(kind)+"/"+stage, func(t *testing.T) {
				n := sampleNote(kind)
				if stage == "render" {
					n.Lines[0].Affectation = "99"
				}
				signErr := errors.New("signing failed")
				calls := 0
				signer := signerFunc(func(doc []byte) ([]byte, error) {
					calls++
					want, err := ubl.Note(n)
					if err != nil || !bytes.Equal(doc, want) {
						t.Error("signer must receive the exact ubl.Note output")
					}
					return []byte("partial signature"), signErr
				})
				tr := &fakeTransport{}
				client := sunat.New(signer, tr)
				check := func(err error) {
					t.Helper()
					want := `sunat: build note: ubl: line 1: catalog: unsupported IGV affectation "99"`
					if stage == "sign" {
						want = "sunat: sign note: signing failed"
						if !errors.Is(err, signErr) {
							t.Errorf("error = %v, want original signer error", err)
						}
					}
					if err == nil || err.Error() != want {
						t.Errorf("error = %v, want %q", err, want)
					}
				}
				out, err := client.BuildNote(n)
				check(err)
				if out != nil {
					t.Error("BuildNote must discard partial XML on failure")
				}
				result, err := client.SendNote(context.Background(), n)
				check(err)
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

func TestSendNote(t *testing.T) {
	signer, _ := newSigner(t)
	for _, kind := range noteTypes {
		for _, code := range []string{"0", "4001", "2000"} {
			t.Run(string(kind)+"/"+code, func(t *testing.T) {
				n := sampleNote(kind)
				zipped, raw := noteCDR(t, n, code)
				tr := &fakeTransport{response: zipped}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				result, err := sunat.New(signer, tr).SendNote(ctx, n)
				if err != nil {
					t.Fatalf("a CDR outcome, including rejection, is not an error: %v", err)
				}
				wantFile := "20000000001-" + string(kind) + "-FC01-3"
				if tr.calls != 1 || tr.fileName != wantFile || tr.ctx != ctx {
					t.Errorf("transport calls=%d filename=%q context passed through=%v", tr.calls, tr.fileName, tr.ctx == ctx)
				}
				wantXML, err := sunat.New(signer, nil).BuildNote(n)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(result.XML, wantXML) || !bytes.Equal(result.XML, tr.xml) {
					t.Error("Result.XML must be exactly the signed XML sent to transport")
				}
				want := cdr.Response{ID: "CDR-1", ReferenceID: "FC01-3", Code: code, Description: "Outcome", Notes: []string{"Observation"}, XML: raw}
				if !reflect.DeepEqual(result.CDR, want) || result.CDR.Accepted() != (code != "2000") {
					t.Errorf("CDR = %+v, want %+v", result.CDR, want)
				}
			})
		}
	}
}

func TestSendNoteErrorsKeepXML(t *testing.T) {
	signer, _ := newSigner(t)
	fault := &transport.Error{Code: "0151", Message: "Invalid ZIP file name", FaultCode: "soap-env:Client.0151"}
	malformed, err := transport.Zip("R-note.xml", []byte("<ApplicationResponse>"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range noteTypes {
		for _, tc := range []struct {
			name     string
			response []byte
			err      error
			prefix   string
		}{
			{"SOAP fault", nil, fault, "sunat: send note: "},
			{"canceled context", nil, context.Canceled, "sunat: send note: "},
			{"invalid ZIP", []byte("not a zip"), nil, "sunat: parse CDR: cdr: open zip: "},
			{"invalid CDR XML", malformed, nil, "sunat: parse CDR: cdr: parse xml: "},
		} {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				tr := &fakeTransport{response: tc.response, err: tc.err}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.err == context.Canceled {
					cancel()
				}
				result, err := sunat.New(signer, tr).SendNote(ctx, sampleNote(kind))
				if err == nil || !strings.HasPrefix(err.Error(), tc.prefix) {
					t.Fatalf("error = %v, want prefix %q", err, tc.prefix)
				}
				if tc.err != nil && !errors.Is(err, tc.err) {
					t.Errorf("error = %v, want wrapped %v", err, tc.err)
				}
				if tc.err == fault {
					var got *transport.Error
					if !errors.As(err, &got) || got != fault {
						t.Errorf("error = %v, want original *transport.Error", err)
					}
				}
				if tc.name == "invalid CDR XML" {
					var got *xml.SyntaxError
					if !errors.As(err, &got) {
						t.Errorf("error = %v, want wrapped *xml.SyntaxError", err)
					}
				}
				if tr.calls != 1 || tr.ctx != ctx || tr.fileName != "20000000001-"+string(kind)+"-FC01-3" || len(result.XML) == 0 || !bytes.Equal(result.XML, tr.xml) {
					t.Error("send/parse errors must preserve signed XML and transport arguments")
				}
				if !reflect.DeepEqual(result.CDR, cdr.Response{}) {
					t.Errorf("CDR = %+v, want empty response", result.CDR)
				}
			})
		}
	}
}

func TestBuildNoteVerifiesWithXmlsec1(t *testing.T) {
	if testing.Short() {
		t.Skip("xmlsec1 integration test")
	}
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	for _, kind := range noteTypes {
		t.Run(string(kind), func(t *testing.T) {
			signer, certPEM := newSigner(t)
			signed, err := sunat.New(signer, nil).BuildNote(sampleNote(kind))
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			doc, crt := filepath.Join(dir, "signed.xml"), filepath.Join(dir, "cert.pem")
			if err := os.WriteFile(crt, certPEM, 0o600); err != nil {
				t.Fatal(err)
			}
			verify := func(data []byte) ([]byte, error) {
				t.Helper()
				if err := os.WriteFile(doc, data, 0o600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(bin, "--verify", "--pubkey-cert-pem", crt, doc)
				cmd.Env = append(os.Environ(), "OPENSSL_ENABLE_SHA1_SIGNATURES=1")
				return cmd.CombinedOutput()
			}
			if out, err := verify(signed); err != nil {
				t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, out)
			}
			for _, target := range []string{"F001-45", "236.00"} {
				tampered := bytes.Replace(signed, []byte(target), []byte("TAMPERED"), 1)
				if bytes.Equal(tampered, signed) {
					t.Fatalf("tamper target %q not found", target)
				}
				if out, err := verify(tampered); err == nil {
					t.Errorf("xmlsec1 accepted tampered %q: %s", target, out)
				}
			}
		})
	}
}
