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

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go"
	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/transport"
	"github.com/villano14/sunat-go/ubl"
)

func sampleReceipt() model.Invoice {
	inv := sampleInvoice()
	inv.Type, inv.Series = catalog.Receipt, "B001"
	inv.Client = model.Client{IdentityType: catalog.DNI, Number: "12345678", Name: "CONSUMIDOR & FAMILIA"}
	// Intentional mismatch with line sums pins manual-amount compatibility,
	// not tax validity. Build/Send must not silently call calc.Totals.
	inv.Payable = decimal.RequireFromString("219.91")
	return inv
}

func receiptCDR(t *testing.T, code string) ([]byte, []byte) {
	t.Helper()
	raw := []byte(`<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"><cbc:ID>CDR-03</cbc:ID><cbc:Note>Observation</cbc:Note><cac:DocumentResponse><cac:Response><cbc:ReferenceID>B001-1</cbc:ReferenceID><cbc:ResponseCode>` + code + `</cbc:ResponseCode><cbc:Description>Outcome</cbc:Description></cac:Response></cac:DocumentResponse></ApplicationResponse>`)
	zipped, err := transport.Zip("R-20000000001-03-B001-1.xml", raw)
	if err != nil {
		t.Fatal(err)
	}
	return zipped, raw
}

func TestBuildReceipt(t *testing.T) {
	signer, _ := newSigner(t)
	inv := sampleReceipt()
	before := sampleReceipt()
	tr := &fakeTransport{}
	got, err := sunat.New(signer, tr).BuildInvoice(inv)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := ubl.Invoice(inv)
	if err != nil {
		t.Fatal(err)
	}
	want, err := signer.Sign(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) || tr.calls != 0 || !reflect.DeepEqual(inv, before) {
		t.Error("BuildInvoice must only render/sign without sending or changing manual input")
	}
	for _, fragment := range []string{
		`<cbc:ID>B001-1</cbc:ID>`,
		`<cbc:InvoiceTypeCode listID="0101">03</cbc:InvoiceTypeCode>`,
		`<cbc:ID schemeID="1">12345678</cbc:ID>`,
		`<cbc:PayableAmount currencyID="PEN">219.91</cbc:PayableAmount>`,
		`<ds:Signature `,
	} {
		if !bytes.Contains(got, []byte(fragment)) {
			t.Errorf("missing signed boleta fragment %s", fragment)
		}
	}
}

func TestSendReceiptCDROutcomes(t *testing.T) {
	signer, _ := newSigner(t)
	for _, tc := range []struct {
		name     string
		code     string
		accepted bool
	}{
		{"accepted", "0", true},
		{"observed", "4001", true},
		{"rejected", "2000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zipped, raw := receiptCDR(t, tc.code)
			tr := &fakeTransport{response: zipped}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			inv, before := sampleReceipt(), sampleReceipt()
			result, err := sunat.New(signer, tr).SendInvoice(ctx, inv)
			if err != nil {
				t.Fatalf("CDR rejection is a response, not a Go error: %v", err)
			}
			if tr.calls != 1 || tr.ctx != ctx || tr.fileName != "20000000001-03-B001-1" {
				t.Errorf("SendBill calls=%d filename=%q context passed=%v", tr.calls, tr.fileName, tr.ctx == ctx)
			}
			wantXML, err := sunat.New(signer, nil).BuildInvoice(inv)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(result.XML, wantXML) || !bytes.Equal(result.XML, tr.xml) || !reflect.DeepEqual(inv, before) {
				t.Error("SendInvoice must retain/send exact signed XML and preserve caller input")
			}
			want := cdr.Response{ID: "CDR-03", ReferenceID: "B001-1", Code: tc.code, Description: "Outcome", Notes: []string{"Observation"}, XML: raw}
			if !reflect.DeepEqual(result.CDR, want) || result.CDR.Accepted() != tc.accepted {
				t.Errorf("CDR=%+v accepted=%v, want %+v accepted=%v", result.CDR, result.CDR.Accepted(), want, tc.accepted)
			}
		})
	}
}

func TestSendReceiptErrorsKeepXML(t *testing.T) {
	signer, _ := newSigner(t)
	fault := &transport.Error{Code: "0151", Message: "Invalid ZIP file name", FaultCode: "soap-env:Client.0151"}
	malformed, err := transport.Zip("R-receipt.xml", []byte("<ApplicationResponse>"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		response []byte
		err      error
		prefix   string
	}{
		{"SOAP fault", nil, fault, "sunat: send invoice: "},
		{"canceled context", nil, context.Canceled, "sunat: send invoice: "},
		{"invalid ZIP", []byte("not a zip"), nil, "sunat: parse CDR: cdr: open zip: "},
		{"invalid CDR XML", malformed, nil, "sunat: parse CDR: cdr: parse xml: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := &fakeTransport{response: tc.response, err: tc.err}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.err == context.Canceled {
				cancel()
			}
			result, err := sunat.New(signer, tr).SendInvoice(ctx, sampleReceipt())
			if err == nil || !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Fatalf("error=%v, want prefix %q", err, tc.prefix)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Errorf("error=%v, want wrapped %v", err, tc.err)
			}
			if tc.err == fault {
				var got *transport.Error
				if !errors.As(err, &got) || got != fault {
					t.Errorf("error=%v, want original *transport.Error", err)
				}
			}
			if tr.calls != 1 || tr.ctx != ctx || tr.fileName != "20000000001-03-B001-1" || len(result.XML) == 0 || !bytes.Equal(result.XML, tr.xml) {
				t.Error("send/parse errors must retain signed XML and SendBill arguments")
			}
			if result.CDR.Accepted() || !reflect.DeepEqual(result.CDR, cdr.Response{}) {
				t.Error("transport success without a parsed CDR must not imply acceptance")
			}
		})
	}
}

func TestBuildReceiptVerifiesWithXmlsec1(t *testing.T) {
	if testing.Short() {
		t.Skip("xmlsec1 integration test")
	}
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	signer, certPEM := newSigner(t)
	signed, err := sunat.New(signer, nil).BuildInvoice(sampleReceipt())
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
		t.Fatalf("xmlsec1 rejected boleta signature: %v\n%s", err, out)
	}
	for _, target := range []string{"B001-1", "219.91"} {
		tampered := bytes.Replace(signed, []byte(target), []byte("TAMPERED"), 1)
		if bytes.Equal(tampered, signed) {
			t.Fatalf("tamper target %q missing", target)
		}
		if out, err := verify(tampered); err == nil {
			t.Errorf("xmlsec1 accepted tampered %q: %s", target, out)
		}
	}
}
