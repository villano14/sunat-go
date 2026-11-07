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
	"time"

	"github.com/villano14/sunat-go/transport"
)

var _ transport.PackTransport = (*transport.SOAPClient)(nil)

// packZIP prepares a real multi-entry archive entirely in memory.
func packZIP(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zw.SetComment("caller-prepared archive"); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, "synthetic fixture for "+name); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSendPackRequest(t *testing.T) {
	for _, fileName := range []string{"20100066603-LT-20160822-1.zip", `pack&<"name.zip`} {
		t.Run(fileName, func(t *testing.T) {
			srv := newBatchServer(t, http.StatusOK, batchSOAPHead+
				`<br:sendPackResponse xmlns:br="http://service.sunat.gob.pe"><ticket> 202600012345 </ticket></br:sendPackResponse>`+batchSOAPTail)
			zipped := packZIP(t, "20100066603-01-F001-1.xml", "20100066603-07-FC01-1.xml")
			before := bytes.Clone(zipped)
			var tr transport.PackTransport = transport.New(srv.URL, creds)
			ticket, err := tr.SendPack(context.Background(), fileName, zipped)
			if err != nil || ticket != "202600012345" {
				t.Fatalf("SendPack = %q, %v", ticket, err)
			}
			var req struct {
				XMLName xml.Name
				Body struct {
					Operation struct {
						XMLName     xml.Name
						FileName    string `xml:"fileName"`
						ContentFile string `xml:"contentFile"`
					} `xml:"http://service.sunat.gob.pe sendPack"`
				} `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
			}
			if err := xml.Unmarshal(srv.body, &req); err != nil {
				t.Fatalf("invalid request XML: %v", err)
			}
			if req.XMLName != (xml.Name{Space: "http://schemas.xmlsoap.org/soap/envelope/", Local: "Envelope"}) ||
				req.Body.Operation.XMLName != (xml.Name{Space: "http://service.sunat.gob.pe", Local: "sendPack"}) {
				t.Fatalf("wrong envelope or operation namespace: %+v", req)
			}
			if req.Body.Operation.FileName != fileName {
				t.Errorf("fileName = %q, want exactly %q (no appended extension)", req.Body.Operation.FileName, fileName)
			}
			decoded, err := base64.StdEncoding.DecodeString(req.Body.Operation.ContentFile)
			if err != nil || !bytes.Equal(decoded, before) || !bytes.Equal(zipped, before) {
				t.Fatalf("archive changed or was recompressed: decode error %v", err)
			}
			if srv.action != `"urn:sendPack"` || srv.contentType != "text/xml; charset=utf-8" || srv.calls != 1 {
				t.Errorf("action=%q contentType=%q calls=%d", srv.action, srv.contentType, srv.calls)
			}
			if a := srv.auth(t); a.Username != creds.RUC+creds.User || a.Password != creds.Password {
				t.Errorf("WS-Security credentials = %+v", a)
			}
		})
	}
}

func TestSendPackMissingTicket(t *testing.T) {
	for name, body := range map[string]string{
		"absent response": "",
		"absent ticket":   `<sendPackResponse/>`,
		"empty ticket":    `<sendPackResponse><ticket/></sendPackResponse>`,
		"blank ticket":    "<sendPackResponse><ticket> \n\t </ticket></sendPackResponse>",
		"wrong operation": `<sendSummaryResponse><ticket>not-a-pack-ticket</ticket></sendSummaryResponse>`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := newBatchServer(t, http.StatusOK, batchSOAPHead+body+batchSOAPTail)
			ticket, err := transport.New(srv.URL, creds).SendPack(context.Background(), "pack.zip", packZIP(t, "one.xml"))
			if ticket != "" || err == nil || err.Error() != "transport: sendPack response has no ticket" || srv.calls != 1 {
				t.Fatalf("ticket=%q error=%v calls=%d", ticket, err, srv.calls)
			}
		})
	}
}

func TestSendPackFault(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		srv := newBatchServer(t, status, batchSOAPHead+
			`<soap-env:Fault><faultcode>soap-env:Client.0151</faultcode><faultstring> ZIP incorrecto </faultstring></soap-env:Fault>`+batchSOAPTail)
		ticket, err := transport.New(srv.URL, creds).SendPack(context.Background(), "pack.zip", packZIP(t, "one.xml"))
		var fault *transport.Error
		if ticket != "" || !errors.As(err, &fault) || fault.Code != "0151" || fault.Message != "ZIP incorrecto" || fault.FaultCode != "soap-env:Client.0151" {
			t.Fatalf("HTTP %d: ticket=%q error=%v", status, ticket, err)
		}
	}
}

func TestSendPackHTTPAndMalformedResponse(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadGateway, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(status)
			w.Write(append([]byte("diagnostic: <unclosed>"), body...))
		}))
		t.Cleanup(srv.Close)
		ticket, err := transport.New(srv.URL, creds).SendPack(context.Background(), "pack.zip", packZIP(t, "one.xml"))
		if ticket != "" || err == nil {
			t.Fatalf("HTTP %d: ticket=%q error=%v", status, ticket, err)
		}
		if strings.Contains(err.Error(), "MODDATOS") || strings.Contains(err.Error(), "moddatos") || strings.Contains(err.Error(), "diagnostic") {
			t.Fatalf("response body leaked: %v", err)
		}
		var httpErr *transport.HTTPError
		if status == http.StatusOK {
			if err.Error() != "transport: unexpected non-SOAP response" || errors.As(err, &httpErr) {
				t.Fatalf("malformed response error = %v", err)
			}
		} else if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
			t.Fatalf("HTTP %d: error identity lost: %v", status, err)
		}
	}
	// A syntactically valid success body must not override an HTTP failure.
	srv := newBatchServer(t, http.StatusBadGateway, batchSOAPHead+`<sendPackResponse><ticket>1</ticket></sendPackResponse>`+batchSOAPTail)
	ticket, err := transport.New(srv.URL, creds).SendPack(context.Background(), "pack.zip", nil)
	var httpErr *transport.HTTPError
	if ticket != "" || !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("ticket=%q error=%v", ticket, err)
	}
}

func TestSendPackContext(t *testing.T) {
	srv := newBatchServer(t, http.StatusOK, "")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, ctx := range []context.Context{canceled, expired} {
		ticket, err := transport.New(srv.URL, creds).SendPack(ctx, "pack.zip", packZIP(t, "one.xml"))
		if ticket != "" || !errors.Is(err, ctx.Err()) {
			t.Fatalf("ticket=%q error=%v, want %v", ticket, err, ctx.Err())
		}
	}
	if srv.calls != 0 {
		t.Fatalf("canceled requests reached server: %d", srv.calls)
	}
}

func TestPackGetStatusRawArchive(t *testing.T) {
	archive := packZIP(t, "R-20100066603-01-F001-1.xml", "R-20100066603-07-FC01-1.xml", "report.txt")
	encoded := base64.StdEncoding.EncodeToString(archive)
	for _, code := range []string{"0", "99"} {
		srv := newBatchServer(t, http.StatusOK, batchStatusResponse("getStatusResponse", "status", code, encoded[:16]+"\n "+encoded[16:]))
		st, err := transport.New(srv.URL, creds).GetStatus(context.Background(), "pack-ticket")
		if err != nil || st.Code != code || !st.Done() || st.WithErrors() != (code == "99") || !bytes.Equal(st.Content, archive) {
			t.Fatalf("code %s: status=%+v error=%v", code, st, err)
		}
	}
	// Keep the decoder error discoverable and reject partial content.
	srv := newBatchServer(t, http.StatusOK, batchStatusResponse("getStatusResponse", "status", "0", "YWJj!!"))
	st, err := transport.New(srv.URL, creds).GetStatus(context.Background(), "pack-ticket")
	var corrupt base64.CorruptInputError
	if !errors.As(err, &corrupt) || st.Code != "" || st.Content != nil {
		t.Fatalf("invalid base64: status=%+v error=%v", st, err)
	}
}

func TestPackPreservesSummaryAndPendingStatus(t *testing.T) {
	// Existing operations keep their own response, ZIP convention and pending state.
	srv := newBatchServer(t, http.StatusOK, batchSOAPHead+
		`<sendPackResponse><ticket>pack-only</ticket></sendPackResponse>`+
		`<sendSummaryResponse><ticket>summary-only</ticket></sendSummaryResponse>`+batchSOAPTail)
	c := transport.New(srv.URL, creds)
	ticket, err := c.SendSummary(context.Background(), "old-summary", []byte("<SummaryDocuments/>"))
	if err != nil || ticket != "summary-only" || srv.action != `"urn:sendSummary"` {
		t.Fatalf("SendSummary = %q, %v; action=%q", ticket, err, srv.action)
	}
	var req struct {
		Name    string `xml:"Body>sendSummary>fileName"`
		Content string `xml:"Body>sendSummary>contentFile"`
	}
	if err := xml.Unmarshal(srv.body, &req); err != nil {
		t.Fatal(err)
	}
	zipped, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		t.Fatal(err)
	}
	name, data := unzipOne(t, zipped)
	if req.Name != "old-summary.zip" || name != "old-summary.xml" || string(data) != "<SummaryDocuments/>" {
		t.Fatalf("SendSummary ZIP convention changed: %q %q %q", req.Name, name, data)
	}
	pending := newBatchServer(t, http.StatusOK, batchStatusResponse("getStatusResponse", "status", "98", ""))
	st, err := transport.New(pending.URL, creds).GetStatus(context.Background(), "summary-ticket")
	if err != nil || st.Code != "98" || !st.InProcess() || st.Done() || st.Content != nil || pending.action != `"urn:getStatus"` {
		t.Fatalf("pending status=%+v error=%v action=%q", st, err, pending.action)
	}
}
