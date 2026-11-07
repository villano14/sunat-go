package transport_test

import (
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

const batchSOAPHead = `<?xml version="1.0" encoding="UTF-8"?><soap-env:Envelope xmlns:soap-env="http://schemas.xmlsoap.org/soap/envelope/"><soap-env:Header/><soap-env:Body>`
const batchSOAPTail = `</soap-env:Body></soap-env:Envelope>`

// batchServer answers every request with body, recording the request.
type batchServer struct {
	*httptest.Server
	action, contentType string
	body                []byte
	calls               int
}

func newBatchServer(t *testing.T, status int, response string) *batchServer {
	t.Helper()
	s := &batchServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.action, s.contentType = r.Header.Get("SOAPAction"), r.Header.Get("Content-Type")
		s.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		io.WriteString(w, response)
	}))
	t.Cleanup(s.Close)
	return s
}

type batchAuth struct {
	Username string `xml:"Header>Security>UsernameToken>Username"`
	Password string `xml:"Header>Security>UsernameToken>Password"`
}

func (s *batchServer) auth(t *testing.T) batchAuth {
	t.Helper()
	var a batchAuth
	if err := xml.Unmarshal(s.body, &a); err != nil {
		t.Fatalf("request is not valid XML: %v\n%s", err, s.body)
	}
	return a
}

func TestSendSummary(t *testing.T) {
	srv := newBatchServer(t, http.StatusOK, batchSOAPHead+
		`<br:sendSummaryResponse xmlns:br="http://service.sunat.gob.pe"><ticket> 202600012345 </ticket></br:sendSummaryResponse>`+batchSOAPTail)
	var got struct {
		FileName    string `xml:"Body>sendSummary>fileName"`
		ContentFile string `xml:"Body>sendSummary>contentFile"`
	}

	ticket, err := transport.New(srv.URL, creds).SendSummary(context.Background(), "20000000001-RC-20261028-1", []byte("<SummaryDocuments/>"))
	if err != nil {
		t.Fatalf("SendSummary() error: %v", err)
	}
	if ticket != "202600012345" {
		t.Errorf("ticket = %q", ticket)
	}
	if err := xml.Unmarshal(srv.body, &got); err != nil {
		t.Fatalf("request is not valid XML: %v\n%s", err, srv.body)
	}
	if got.FileName != "20000000001-RC-20261028-1.zip" {
		t.Errorf("fileName = %q", got.FileName)
	}
	zipped, err := base64.StdEncoding.DecodeString(got.ContentFile)
	if err != nil {
		t.Fatalf("contentFile is not base64: %v", err)
	}
	name, data := unzipOne(t, zipped)
	if name != "20000000001-RC-20261028-1.xml" || string(data) != "<SummaryDocuments/>" {
		t.Errorf("zip entry = %q %q", name, data)
	}
	if a := srv.auth(t); a.Username != "20000000001MODDATOS" || a.Password != "moddatos&<" {
		t.Errorf("credentials = %+v", a)
	}
	if srv.action != `"urn:sendSummary"` || !strings.HasPrefix(srv.contentType, "text/xml") {
		t.Errorf("headers SOAPAction=%q Content-Type=%q", srv.action, srv.contentType)
	}
	if !strings.Contains(string(srv.body), `<ser:sendSummary>`) {
		t.Errorf("request must use the ser:sendSummary operation:\n%s", srv.body)
	}
}

func TestSendSummaryMissingTicket(t *testing.T) {
	for name, response := range map[string]string{
		"no element":   batchSOAPHead + `<br:sendSummaryResponse xmlns:br="http://service.sunat.gob.pe"/>` + batchSOAPTail,
		"other action": batchSOAPHead + `<br:sendBillResponse xmlns:br="http://service.sunat.gob.pe"/>` + batchSOAPTail,
	} {
		t.Run(name, func(t *testing.T) {
			srv := newBatchServer(t, http.StatusOK, response)
			if _, err := transport.New(srv.URL, creds).SendSummary(context.Background(), "x", []byte("<a/>")); err == nil {
				t.Fatal("expected an error for a response without a ticket")
			}
		})
	}
}

func batchStatusResponse(op, element, code, content string) string {
	s := batchSOAPHead + `<br:` + op + ` xmlns:br="http://service.sunat.gob.pe"><` + element + `>`
	if content != "" {
		s += `<content>` + content + `</content>`
	}
	return s + `<statusCode>` + code + `</statusCode></` + element + `></br:` + op + `>` + batchSOAPTail
}

func TestGetStatus(t *testing.T) {
	cdrZip := []byte("cdr-zip-bytes")
	b64 := base64.StdEncoding.EncodeToString(cdrZip)
	wrapped := b64[:8] + "\n" + b64[8:] // SOAP stacks may wrap base64 lines
	tests := []struct {
		name, code, content   string
		inProcess, done, errs bool
		wantContent           []byte
	}{
		{"processed", "0", wrapped, false, true, false, cdrZip},
		{"in process", "98", "", true, false, false, nil},
		{"with errors", "99", b64, false, true, true, cdrZip},
		{"padded code", "0098", "", true, false, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newBatchServer(t, http.StatusOK, batchStatusResponse("getStatusResponse", "status", tt.code, tt.content))
			st, err := transport.New(srv.URL, creds).GetStatus(context.Background(), "2026&1")
			if err != nil {
				t.Fatalf("GetStatus() error: %v", err)
			}
			if st.Code != tt.code || string(st.Content) != string(tt.wantContent) {
				t.Errorf("Status = %+v", st)
			}
			if st.InProcess() != tt.inProcess || st.Done() != tt.done || st.WithErrors() != tt.errs {
				t.Errorf("InProcess=%v Done=%v WithErrors=%v", st.InProcess(), st.Done(), st.WithErrors())
			}
			var req struct {
				Ticket string `xml:"Body>getStatus>ticket"`
			}
			if err := xml.Unmarshal(srv.body, &req); err != nil || req.Ticket != "2026&1" {
				t.Errorf("ticket = %q (%v)", req.Ticket, err)
			}
			if srv.action != `"urn:getStatus"` {
				t.Errorf("SOAPAction = %q", srv.action)
			}
			if a := srv.auth(t); a.Username != "20000000001MODDATOS" || a.Password != "moddatos&<" {
				t.Errorf("credentials = %+v", a)
			}
		})
	}
}

func TestGetStatusMalformed(t *testing.T) {
	for name, response := range map[string]string{
		"no status":      batchSOAPHead + `<br:getStatusResponse xmlns:br="http://service.sunat.gob.pe"/>` + batchSOAPTail,
		"no status code": batchSOAPHead + `<br:getStatusResponse xmlns:br="http://service.sunat.gob.pe"><status/></br:getStatusResponse>` + batchSOAPTail,
		"bad base64":     batchStatusResponse("getStatusResponse", "status", "0", "!!not-base64!!"),
	} {
		t.Run(name, func(t *testing.T) {
			srv := newBatchServer(t, http.StatusOK, response)
			_, err := transport.New(srv.URL, creds).GetStatus(context.Background(), "1")
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), "not-base64") {
				t.Errorf("error leaks the response content: %v", err)
			}
		})
	}
}

func TestGetStatusRequiresTicket(t *testing.T) {
	srv := newBatchServer(t, http.StatusOK, "")
	if _, err := transport.New(srv.URL, creds).GetStatus(context.Background(), "  "); err == nil || srv.calls != 0 {
		t.Fatalf("error = %v, calls = %d; want a local error without calling SUNAT", err, srv.calls)
	}
}

func TestGetStatusCdr(t *testing.T) {
	cdrZip := []byte("cdr-zip-bytes")
	response := batchSOAPHead + `<br:getStatusCdrResponse xmlns:br="http://service.sunat.gob.pe"><statusCdr><content>` +
		base64.StdEncoding.EncodeToString(cdrZip) +
		`</content><statusCode>0004</statusCode><statusMessage> El comprobante existe </statusMessage></statusCdr></br:getStatusCdrResponse>` + batchSOAPTail
	srv := newBatchServer(t, http.StatusOK, response)
	// The bill endpoint is unused: consult must go to its own endpoint.
	c := transport.New("http://127.0.0.1:1/billService", creds, transport.WithConsultEndpoint(srv.URL))

	st, err := c.GetStatusCdr(context.Background(), "20000000001", "01", "F001", 12)
	if err != nil {
		t.Fatalf("GetStatusCdr() error: %v", err)
	}
	if st.Code != "0004" || st.Message != "El comprobante existe" || string(st.Content) != string(cdrZip) {
		t.Errorf("Status = %+v", st)
	}
	var req struct {
		RUC    string `xml:"Body>getStatusCdr>rucComprobante"`
		Type   string `xml:"Body>getStatusCdr>tipoComprobante"`
		Series string `xml:"Body>getStatusCdr>serieComprobante"`
		Number string `xml:"Body>getStatusCdr>numeroComprobante"`
	}
	if err := xml.Unmarshal(srv.body, &req); err != nil {
		t.Fatalf("request is not valid XML: %v\n%s", err, srv.body)
	}
	if req.RUC != "20000000001" || req.Type != "01" || req.Series != "F001" || req.Number != "12" {
		t.Errorf("request = %+v", req)
	}
	if srv.action != `"urn:getStatusCdr"` {
		t.Errorf("SOAPAction = %q", srv.action)
	}
	if a := srv.auth(t); a.Username != "20000000001MODDATOS" || a.Password != "moddatos&<" {
		t.Errorf("credentials = %+v", a)
	}
}

func TestGetStatusCdrWithoutContent(t *testing.T) {
	srv := newBatchServer(t, http.StatusOK, batchSOAPHead+
		`<br:getStatusCdrResponse xmlns:br="http://service.sunat.gob.pe"><statusCdr><statusCode>0011</statusCode><statusMessage>El comprobante de pago electrónico no existe</statusMessage></statusCdr></br:getStatusCdrResponse>`+batchSOAPTail)
	st, err := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL)).GetStatusCdr(context.Background(), "20000000001", "01", "F001", 1)
	if err != nil {
		t.Fatalf("GetStatusCdr() error: %v", err)
	}
	if st.Code != "0011" || len(st.Content) != 0 || !strings.Contains(st.Message, "no existe") {
		t.Errorf("Status = %+v", st)
	}
}

func TestGetStatusCdrEndpoints(t *testing.T) {
	if transport.ConsultProduction != "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService" {
		t.Errorf("ConsultProduction = %q", transport.ConsultProduction)
	}
	// Beta has no consult service: fail locally instead of calling billService.
	for _, endpoint := range []string{transport.BillBeta, transport.OtrosCPEBeta} {
		_, err := transport.New(endpoint, creds).GetStatusCdr(context.Background(), "20000000001", "01", "F001", 1)
		if !errors.Is(err, transport.ErrNoConsultEndpoint) {
			t.Errorf("%s: error = %v, want ErrNoConsultEndpoint", endpoint, err)
		}
	}
}

func TestGetStatusCdrRejectsBadNumber(t *testing.T) {
	srv := newBatchServer(t, http.StatusOK, "")
	c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
	for _, n := range []int64{0, -1, 1 << 31} {
		if _, err := c.GetStatusCdr(context.Background(), "20000000001", "01", "F001", n); err == nil {
			t.Errorf("number %d: expected an error", n)
		}
	}
	if srv.calls != 0 {
		t.Errorf("calls = %d, want 0", srv.calls)
	}
}

func TestBatchSOAPFault(t *testing.T) {
	srv := faultServer(t, "soap-env:Client.0127", "El ticket no existe")
	defer srv.Close()
	c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
	calls := map[string]func() error{
		"SendSummary": func() error {
			_, err := c.SendSummary(context.Background(), "x", []byte("<a/>"))
			return err
		},
		"GetStatus": func() error {
			_, err := c.GetStatus(context.Background(), "1")
			return err
		},
		"GetStatusCdr": func() error {
			_, err := c.GetStatusCdr(context.Background(), "20000000001", "01", "F001", 1)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var sErr *transport.Error
			if err := call(); !errors.As(err, &sErr) || sErr.Code != "0127" || sErr.Message != "El ticket no existe" {
				t.Fatalf("error = %v, want *transport.Error with code 0127", err)
			}
		})
	}
}

func TestBatchHTTPErrorHidesBody(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadGateway} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(status)
			w.Write(append([]byte("diagnostic: "), body...))
		}))
		c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
		calls := map[string]func() error{
			"SendSummary": func() error {
				_, err := c.SendSummary(context.Background(), "x", []byte("<a/>"))
				return err
			},
			"GetStatus": func() error {
				_, err := c.GetStatus(context.Background(), "1")
				return err
			},
			"GetStatusCdr": func() error {
				_, err := c.GetStatusCdr(context.Background(), "20000000001", "01", "F001", 1)
				return err
			},
		}
		for name, call := range calls {
			err := call()
			if err == nil {
				t.Fatalf("%s status %d: expected error", name, status)
			}
			if strings.Contains(err.Error(), creds.Password) || strings.Contains(err.Error(), "MODDATOS") {
				t.Errorf("%s status %d: error exposes credentials: %v", name, status, err)
			}
			var hErr *transport.HTTPError
			if status != http.StatusOK && (!errors.As(err, &hErr) || hErr.StatusCode != status) {
				t.Errorf("%s status %d: error = %v, want *transport.HTTPError", name, status, err)
			}
		}
		srv.Close()
	}
}

func TestBatchContextCanceled(t *testing.T) {
	srv := newBatchServer(t, http.StatusOK, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
	if _, err := c.SendSummary(ctx, "x", []byte("<a/>")); !errors.Is(err, context.Canceled) {
		t.Errorf("SendSummary error = %v", err)
	}
	if _, err := c.GetStatus(ctx, "1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetStatus error = %v", err)
	}
	if _, err := c.GetStatusCdr(ctx, "20000000001", "01", "F001", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("GetStatusCdr error = %v", err)
	}
}

func TestStatusHelpers(t *testing.T) {
	for _, tt := range []struct {
		code                  string
		inProcess, done, errs bool
	}{
		{"0", false, true, false},
		{"98", true, false, false},
		{"99", false, true, true},
		{"0004", false, false, false},
		{"", false, false, false},
		{"abc", false, false, false},
	} {
		s := transport.Status{Code: tt.code}
		if s.InProcess() != tt.inProcess || s.Done() != tt.done || s.WithErrors() != tt.errs {
			t.Errorf("code %q: InProcess=%v Done=%v WithErrors=%v", tt.code, s.InProcess(), s.Done(), s.WithErrors())
		}
	}
}

// The SOAPClient must keep satisfying every transport interface.
var (
	_ transport.Transport        = (*transport.SOAPClient)(nil)
	_ transport.BatchTransport   = (*transport.SOAPClient)(nil)
	_ transport.ConsultTransport = (*transport.SOAPClient)(nil)
)
