package transport_test

import (
	"context"
	"encoding/xml"
	"errors"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/villano14/sunat-go/transport"
)

var _ transport.CPEStatusTransport = (*transport.SOAPClient)(nil)

func TestGetStatusCpe(t *testing.T) {
	for _, code := range []string{"0001", "0002", "0003", "0011", "0099", "0", "1"} {
		t.Run(code, func(t *testing.T) {
			response := batchSOAPHead + `<getStatusResponse><status><content>not-a-CDR-or-base64</content><statusCode>` + code + `</statusCode><statusMessage> El comprobante &amp; estado </statusMessage></status></getStatusResponse>` + batchSOAPTail
			srv := newBatchServer(t, http.StatusOK, response)
			c := transport.New("http://127.0.0.1:1/unused-bill", creds, transport.WithConsultEndpoint(srv.URL))
			st, err := c.GetStatusCpe(context.Background(), "20000000001", "01", "F022&<", 7782)
			if err != nil {
				t.Fatal(err)
			}
			if st.Code != code || st.Message != "El comprobante & estado" {
				t.Fatalf("status = %+v", st)
			}
			var req struct {
				RUC    string  `xml:"Body>getStatus>rucComprobante"`
				Type   string  `xml:"Body>getStatus>tipoComprobante"`
				Series string  `xml:"Body>getStatus>serieComprobante"`
				Number string  `xml:"Body>getStatus>numeroComprobante"`
				Ticket *string `xml:"Body>getStatus>ticket"`
			}
			if err := xml.Unmarshal(srv.body, &req); err != nil {
				t.Fatal(err)
			}
			if req.RUC != "20000000001" || req.Type != "01" || req.Series != "F022&<" || req.Number != "7782" || req.Ticket != nil {
				t.Fatalf("request = %+v", req)
			}
			if srv.action != `"urn:getStatus"` || srv.contentType != "text/xml; charset=utf-8" || srv.calls != 1 {
				t.Fatalf("headers/calls = %q %q %d", srv.action, srv.contentType, srv.calls)
			}
			if a := srv.auth(t); a.Username != creds.RUC+creds.User || a.Password != creds.Password {
				t.Fatalf("auth = %+v", a)
			}
		})
	}
}

func TestGetStatusCpeErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"missing status", batchSOAPHead + `<getStatusResponse/>` + batchSOAPTail, "transport: getStatusCpe response has no status", 200},
		{"missing code", batchSOAPHead + `<getStatusResponse><status><statusMessage>x</statusMessage></status></getStatusResponse>` + batchSOAPTail, "transport: getStatusCpe response has no statusCode", 200},
		{"malformed", "not XML", "transport: unexpected non-SOAP response", 200},
		{"HTTP", "proxy diagnostic", "transport: HTTP 502", 502},
		{"fault", batchSOAPHead + `<Fault><faultcode>soap-env:Client.0125</faultcode><faultstring>No se pudo consultar</faultstring></Fault>` + batchSOAPTail, "sunat: 0125: No se pudo consultar", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newBatchServer(t, tc.status, tc.body)
			c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
			st, err := c.GetStatusCpe(context.Background(), "20000000001", "01", "F001", 1)
			if err == nil || err.Error() != tc.want || !reflect.DeepEqual(st, transport.CPEStatus{}) {
				t.Fatalf("status=%+v error=%v", st, err)
			}
			var he *transport.HTTPError
			var se *transport.Error
			if tc.name == "HTTP" && (!errors.As(err, &he) || he.StatusCode != 502) {
				t.Fatalf("HTTP type lost: %v", err)
			}
			if tc.name == "fault" && (!errors.As(err, &se) || se.Code != "0125") {
				t.Fatalf("fault type lost: %v", err)
			}
		})
	}
	srv := newBatchServer(t, 200, batchStatusResponse("getStatusResponse", "status", "0001", ""))
	c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
	for _, n := range []int64{-1, 0, math.MaxInt32 + 1} {
		_, err := c.GetStatusCpe(context.Background(), "20000000001", "01", "F001", n)
		if err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Fatalf("number %d: %v", n, err)
		}
	}
	if srv.calls != 0 {
		t.Fatalf("invalid input caused %d requests", srv.calls)
	}
	for _, n := range []int64{1, math.MaxInt32} {
		if _, err := c.GetStatusCpe(context.Background(), "20000000001", "01", "F001", n); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetStatusCpe(ctx, "20000000001", "01", "F001", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("context: %v", err)
	}
	for _, endpoint := range []string{transport.BillBeta, transport.OtrosCPEBeta} {
		if _, err := transport.New(endpoint, creds).GetStatusCpe(context.Background(), "20000000001", "01", "F001", 1); !errors.Is(err, transport.ErrNoConsultEndpoint) {
			t.Fatalf("beta: %v", err)
		}
	}
}

func TestConsultAndTicketContentCompatibility(t *testing.T) {
	// CDR recovery and ticket polling must still decode content, unlike the
	// new status-only operation, which intentionally ignores it.
	for _, op := range []string{"getStatusCdr", "getStatus"} {
		element := "status"
		if op == "getStatusCdr" {
			element = "statusCdr"
		}
		srv := newBatchServer(t, 200, batchStatusResponse(op+"Response", element, "0001", "not-base64"))
		c := transport.New(srv.URL, creds, transport.WithConsultEndpoint(srv.URL))
		var err error
		if op == "getStatusCdr" {
			_, err = c.GetStatusCdr(context.Background(), "20000000001", "01", "F001", 1)
		} else {
			_, err = c.GetStatus(context.Background(), "ticket")
		}
		if err == nil || !strings.HasPrefix(err.Error(), "transport: decode "+op+" content: ") {
			t.Fatalf("%s: error=%v", op, err)
		}
	}
}
