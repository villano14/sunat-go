package transport_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/villano14/sunat-go/transport"
)

var creds = transport.Credentials{RUC: "20000000001", User: "MODDATOS", Password: "moddatos&<"}

type sendBillRequest struct {
	Username    string `xml:"Header>Security>UsernameToken>Username"`
	Password    string `xml:"Header>Security>UsernameToken>Password"`
	FileName    string `xml:"Body>sendBill>fileName"`
	ContentFile string `xml:"Body>sendBill>contentFile"`
}

func unzipOne(t *testing.T, b []byte) (string, []byte) {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	if len(r.File) != 1 {
		t.Fatalf("zip has %d files, want 1", len(r.File))
	}
	f, err := r.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	return r.File[0].Name, data
}

func TestSendBill(t *testing.T) {
	cdr := []byte("cdr-zip-bytes")
	var got sendBillRequest
	var soapAction, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		soapAction, contentType = r.Header.Get("SOAPAction"), r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if err := xml.Unmarshal(body, &got); err != nil {
			t.Errorf("request is not valid XML: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><soap-env:Envelope xmlns:soap-env="http://schemas.xmlsoap.org/soap/envelope/"><soap-env:Header/><soap-env:Body><br:sendBillResponse xmlns:br="http://service.sunat.gob.pe"><applicationResponse>`+
			base64.StdEncoding.EncodeToString(cdr)+`</applicationResponse></br:sendBillResponse></soap-env:Body></soap-env:Envelope>`)
	}))
	defer srv.Close()

	c := transport.New(srv.URL, creds)
	res, err := c.SendBill(context.Background(), "20000000001-01-F001-1", []byte("<Invoice/>"))
	if err != nil {
		t.Fatalf("SendBill() error: %v", err)
	}
	if !bytes.Equal(res, cdr) {
		t.Errorf("SendBill() = %q, want %q", res, cdr)
	}
	if got.Username != "20000000001MODDATOS" || got.Password != "moddatos&<" {
		t.Errorf("credentials = %q / %q", got.Username, got.Password)
	}
	if got.FileName != "20000000001-01-F001-1.zip" {
		t.Errorf("fileName = %q", got.FileName)
	}
	zipped, err := base64.StdEncoding.DecodeString(got.ContentFile)
	if err != nil {
		t.Fatalf("contentFile is not base64: %v", err)
	}
	name, data := unzipOne(t, zipped)
	if name != "20000000001-01-F001-1.xml" || string(data) != "<Invoice/>" {
		t.Errorf("zip entry = %q %q", name, data)
	}
	if soapAction != `"urn:sendBill"` || !strings.HasPrefix(contentType, "text/xml") {
		t.Errorf("headers SOAPAction=%q Content-Type=%q", soapAction, contentType)
	}
}

func faultServer(t *testing.T, faultcode, faultstring string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `<soap-env:Envelope xmlns:soap-env="http://schemas.xmlsoap.org/soap/envelope/"><soap-env:Body><soap-env:Fault><faultcode>`+
			faultcode+`</faultcode><faultstring>`+faultstring+`</faultstring></soap-env:Fault></soap-env:Body></soap-env:Envelope>`)
	}))
}

func TestSendBillFault(t *testing.T) {
	tests := []struct {
		faultcode, faultstring, code, message string
	}{
		{"soap-env:Client.0151", "El nombre del archivo ZIP es incorrecto", "0151", "El nombre del archivo ZIP es incorrecto"},
		{"soap-env:Client", "0102", "0102", "0102"},
		{"soap-env:Server", "Internal Error", "", "Internal Error"},
	}
	for _, tt := range tests {
		t.Run(tt.faultcode, func(t *testing.T) {
			srv := faultServer(t, tt.faultcode, tt.faultstring)
			defer srv.Close()
			_, err := transport.New(srv.URL, creds).SendBill(context.Background(), "x", []byte("<a/>"))
			var sErr *transport.Error
			if !errors.As(err, &sErr) {
				t.Fatalf("error = %v, want *transport.Error", err)
			}
			if sErr.Code != tt.code || sErr.Message != tt.message || sErr.FaultCode != tt.faultcode {
				t.Errorf("Error = %+v, want code %q message %q", sErr, tt.code, tt.message)
			}
			if !strings.Contains(err.Error(), tt.faultstring) {
				t.Errorf("Error() = %q, missing fault string", err.Error())
			}
		})
	}
}

func TestSendBillNonSOAPResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "<html>bad gateway</html>")
	}))
	defer srv.Close()
	_, err := transport.New(srv.URL, creds).SendBill(context.Background(), "x", []byte("<a/>"))
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v, want HTTP 502 error", err)
	}
	var sErr *transport.Error
	if errors.As(err, &sErr) {
		t.Error("non-SOAP response must not be a SUNAT *transport.Error")
	}
}

func TestSendBillContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := transport.New(srv.URL, creds).SendBill(ctx, "x", []byte("<a/>"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestZip(t *testing.T) {
	b, err := transport.Zip("F001-1.xml", []byte("<a/>"))
	if err != nil {
		t.Fatal(err)
	}
	name, data := unzipOne(t, b)
	if name != "F001-1.xml" || string(data) != "<a/>" {
		t.Errorf("zip entry = %q %q", name, data)
	}
}

func TestEndpoints(t *testing.T) {
	if transport.BillBeta != "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService" {
		t.Errorf("BillBeta = %q", transport.BillBeta)
	}
	if transport.BillProduction != "https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService" {
		t.Errorf("BillProduction = %q", transport.BillProduction)
	}
}
