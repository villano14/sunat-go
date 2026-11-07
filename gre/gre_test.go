package gre_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/villano14/sunat-go/gre"
	"github.com/villano14/sunat-go/transport"
)

const (
	testClientID     = "test-client-id"
	testClientSecret = "s3cr3t-client-value"
	testPassword     = "p4ssw0rd-sol-value"
	testFileName     = "20000000001-09-T001-1"
)

var testCreds = gre.Credentials{
	ClientID: testClientID, ClientSecret: testClientSecret,
	RUC: "20000000001", User: "MODDATOS", Password: testPassword,
}

// fakeSUNAT is a scriptable stand-in for both SUNAT services.
type fakeSUNAT struct {
	srv *httptest.Server

	mu     sync.Mutex
	tokens int // token requests served
	issued []string
	// valid is the set of tokens the API accepts; others get 401.
	valid map[string]bool

	tokenCalls atomic.Int32
	apiCalls   atomic.Int32

	expiresIn   int
	tokenStatus int
	tokenBody   string // served with tokenStatus when it is not 200
	tokenOK     string // replaces the normal 200 token response when set
	omitExpires bool   // 200 token responses carry no expires_in
	rejectAll   bool   // the API answers 401 to every token
	tokenDelay  time.Duration

	apiHandler func(w http.ResponseWriter, r *http.Request)

	lastTokenForm map[string][]string
	lastTokenPath string
	lastAPI       *http.Request
	lastAPIBody   []byte
	bodies        [][]byte // body of every API request, including rejected ones
}

func newFakeSUNAT(t *testing.T) *fakeSUNAT {
	t.Helper()
	f := &fakeSUNAT{valid: map[string]bool{}, expiresIn: 3600, tokenStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/sec/", f.serveToken)
	mux.HandleFunc("/api/", f.serveAPI)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSUNAT) serveToken(w http.ResponseWriter, r *http.Request) {
	f.tokenCalls.Add(1)
	if f.tokenDelay > 0 {
		time.Sleep(f.tokenDelay)
	}
	_ = r.ParseForm()
	f.mu.Lock()
	f.lastTokenForm = r.PostForm
	f.lastTokenPath = r.Method + " " + r.URL.Path + " " + r.Header.Get("Content-Type")
	if f.tokenStatus != http.StatusOK {
		status, body := f.tokenStatus, f.tokenBody
		f.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
		return
	}
	if f.tokenOK != "" {
		body := f.tokenOK
		f.mu.Unlock()
		_, _ = io.WriteString(w, body)
		return
	}
	f.tokens++
	tok := fmt.Sprintf("token-%d", f.tokens)
	f.issued = append(f.issued, tok)
	f.valid[tok] = true
	expires := f.expiresIn
	omit := f.omitExpires
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if omit {
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer"}`, tok)
		return
	}
	_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":%d}`, tok, expires)
}

func (f *fakeSUNAT) serveAPI(w http.ResponseWriter, r *http.Request) {
	f.apiCalls.Add(1)
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.lastAPI, f.lastAPIBody = r, body
	f.bodies = append(f.bodies, body)
	ok := !f.rejectAll && f.valid[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"cod":"401","msg":"Unauthorized"}`)
		return
	}
	f.apiHandler(w, r)
}

func (f *fakeSUNAT) client(opts ...gre.Option) *gre.Client {
	opts = append([]gre.Option{gre.WithEndpoints(f.srv.URL+"/sec", f.srv.URL+"/api/")}, opts...)
	return gre.New(testCreds, opts...)
}

func respond(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func TestSend(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{"numTicket":"11111111-2222-3333-4444-555555555555","fecRecepcion":"2026-11-05T10:15:30.000-05:00"}`)
	xml := []byte(`<DespatchAdvice>signed</DespatchAdvice>`)

	ticket, err := f.client().Send(context.Background(), testFileName, xml)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if ticket.Number != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("ticket = %q", ticket.Number)
	}
	if want := time.Date(2026, 11, 5, 10, 15, 30, 0, time.FixedZone("", -5*3600)); !ticket.ReceivedAt.Equal(want) {
		t.Errorf("ReceivedAt = %v, want %v", ticket.ReceivedAt, want)
	}

	form := f.lastTokenForm
	for key, want := range map[string]string{
		"grant_type": "password", "scope": "https://api-cpe.sunat.gob.pe",
		"client_id": testClientID, "client_secret": testClientSecret,
		"username": "20000000001MODDATOS", "password": testPassword,
	} {
		if got := form[key]; len(got) != 1 || got[0] != want {
			t.Errorf("token form %s = %q, want %q", key, got, want)
		}
	}
	if want := "POST /sec/clientessol/test-client-id/oauth2/token/ application/x-www-form-urlencoded"; f.lastTokenPath != want {
		t.Errorf("token request = %q, want %q", f.lastTokenPath, want)
	}

	req := f.lastAPI
	if req.Method != http.MethodPost || req.URL.Path != "/api/contribuyente/gem/comprobantes/"+testFileName {
		t.Errorf("send request = %s %s", req.Method, req.URL.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer token-1" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	var payload struct {
		Archivo struct {
			NomArchivo string `json:"nomArchivo"`
			ArcGreZip  string `json:"arcGreZip"`
			HashZip    string `json:"hashZip"`
		} `json:"archivo"`
	}
	if err := json.Unmarshal(f.lastAPIBody, &payload); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if payload.Archivo.NomArchivo != testFileName+".zip" {
		t.Errorf("nomArchivo = %q", payload.Archivo.NomArchivo)
	}
	zipped, err := base64.StdEncoding.DecodeString(payload.Archivo.ArcGreZip)
	if err != nil {
		t.Fatalf("arcGreZip is not base64: %v", err)
	}
	sum := sha256.Sum256(zipped)
	if payload.Archivo.HashZip != hex.EncodeToString(sum[:]) {
		t.Errorf("hashZip = %q, want sha256 hex of the zip", payload.Archivo.HashZip)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil || len(zr.File) != 1 || zr.File[0].Name != testFileName+".xml" {
		t.Fatalf("zip contents unexpected: %v", err)
	}
	rc, _ := zr.File[0].Open()
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, xml) {
		t.Errorf("zipped xml = %q", got)
	}
}

func TestSendPathEscapesFileName(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{"numTicket":"t"}`)
	if _, err := f.client().Send(context.Background(), "a/b c", []byte("<x/>")); err != nil {
		t.Fatal(err)
	}
	if got := f.lastAPI.URL.EscapedPath(); got != "/api/contribuyente/gem/comprobantes/a%2Fb%20c" {
		t.Errorf("path = %q", got)
	}
}

func cdrZip(t *testing.T) []byte {
	t.Helper()
	z, err := transport.Zip("R-"+testFileName+".xml", []byte("<ApplicationResponse/>"))
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func TestStatus(t *testing.T) {
	zipped := cdrZip(t)
	tests := []struct {
		name string
		body string
		want func(t *testing.T, s gre.Status)
	}{
		{
			name: "accepted",
			body: fmt.Sprintf(`{"codRespuesta":"0","arcCdr":%q,"indCdrGenerado":"1"}`, base64.StdEncoding.EncodeToString(zipped)),
			want: func(t *testing.T, s gre.Status) {
				if !s.Accepted() || s.InProcess() || s.Rejected() || !s.CDRGenerated || !bytes.Equal(s.CDRZip, zipped) || s.Error != nil {
					t.Errorf("status = %+v", s)
				}
			},
		},
		{
			name: "numeric codes",
			body: `{"codRespuesta":98,"indCdrGenerado":0}`,
			want: func(t *testing.T, s gre.Status) {
				if !s.InProcess() || s.CDRGenerated || s.CDRZip != nil {
					t.Errorf("status = %+v", s)
				}
			},
		},
		{
			name: "rejected is a status, not an error",
			body: `{"codRespuesta":"99","error":{"numError":"2335","desError":"El documento electronico ingresado ha sido alterado"},"indCdrGenerado":"0"}`,
			want: func(t *testing.T, s gre.Status) {
				if !s.Rejected() || s.Error == nil || s.Error.Code != "2335" ||
					s.Error.Message != "El documento electronico ingresado ha sido alterado" || s.Error.HTTPStatus != 200 {
					t.Errorf("status = %+v error=%+v", s, s.Error)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSUNAT(t)
			f.apiHandler = respond(200, tt.body)
			s, err := f.client().Status(context.Background(), "ticket-1")
			if err != nil {
				t.Fatalf("Status() error: %v", err)
			}
			tt.want(t, s)
			if f.lastAPI.Method != http.MethodGet || f.lastAPI.URL.Path != "/api/contribuyente/gem/comprobantes/envios/ticket-1" {
				t.Errorf("status request = %s %s", f.lastAPI.Method, f.lastAPI.URL.Path)
			}
			if got := f.lastAPI.Header.Get("Authorization"); got != "Bearer token-1" {
				t.Errorf("Authorization = %q", got)
			}
		})
	}
}

func TestStatusProtocolErrors(t *testing.T) {
	for name, body := range map[string]string{
		"unexpected code": `{"codRespuesta":"7"}`,
		"missing code":    `{}`,
		"bad base64":      `{"codRespuesta":"0","arcCdr":"!!!not-base64!!!"}`,
		"not json":        `<html>oops</html>`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeSUNAT(t)
			f.apiHandler = respond(200, body)
			if _, err := f.client().Status(context.Background(), "t"); err == nil {
				t.Fatal("expected error")
			} else if strings.Contains(err.Error(), "oops") {
				t.Errorf("error leaks the response body: %v", err)
			}
		})
	}
	f := newFakeSUNAT(t)
	if _, err := f.client().Status(context.Background(), ""); err == nil {
		t.Error("expected error for empty ticket")
	}
	if f.tokenCalls.Load() != 0 {
		t.Error("an invalid ticket must not trigger I/O")
	}
}

func TestSendMissingTicket(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{"fecRecepcion":"2026-11-05T10:15:30"}`)
	if _, err := f.client().Send(context.Background(), testFileName, []byte("<x/>")); err == nil {
		t.Fatal("expected error")
	}
}

func TestTokenIsCachedAndRenewedBeforeExpiry(t *testing.T) {
	f := newFakeSUNAT(t)
	f.expiresIn = 3600
	f.apiHandler = respond(200, `{"codRespuesta":"98"}`)
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	c := f.client(gre.WithClock(func() time.Time { return now }))
	status := func(wantAuth string) {
		t.Helper()
		if _, err := c.Status(context.Background(), "t"); err != nil {
			t.Fatalf("Status() error: %v", err)
		}
		if got := f.lastAPI.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want %q", got, wantAuth)
		}
	}

	status("Bearer token-1")
	now = now.Add(49 * time.Minute) // still before expiry minus the 10 minute margin
	status("Bearer token-1")
	if n := f.tokenCalls.Load(); n != 1 {
		t.Fatalf("token requests = %d, want 1 while the token is fresh", n)
	}
	now = now.Add(2 * time.Minute) // 51 minutes in: inside the margin
	status("Bearer token-2")
	if n := f.tokenCalls.Load(); n != 2 {
		t.Fatalf("token requests = %d, want 2 after renewal", n)
	}
}

func TestMissingExpiresInDefaultsToOneHour(t *testing.T) {
	f := newFakeSUNAT(t)
	f.omitExpires = true
	f.apiHandler = respond(200, `{"codRespuesta":"98"}`)
	now := time.Now()
	c := f.client(gre.WithClock(func() time.Time { return now }))
	for i := 0; i < 3; i++ {
		if _, err := c.Status(context.Background(), "t"); err != nil {
			t.Fatal(err)
		}
		now = now.Add(10 * time.Minute)
	}
	if n := f.tokenCalls.Load(); n != 1 {
		t.Errorf("token requests = %d, want 1", n)
	}
	now = now.Add(time.Hour)
	if _, err := c.Status(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	if n := f.tokenCalls.Load(); n != 2 {
		t.Errorf("token requests = %d, want 2 after the default hour", n)
	}
}

func TestRetriesOnceOn401(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{"codRespuesta":"98"}`)
	c := f.client()
	if _, err := c.Status(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	// SUNAT revokes the token early: the cached one now gets 401.
	f.mu.Lock()
	f.valid = map[string]bool{}
	f.mu.Unlock()

	if _, err := c.Status(context.Background(), "t"); err != nil {
		t.Fatalf("Status() after revocation error: %v", err)
	}
	if got := f.lastAPI.Header.Get("Authorization"); got != "Bearer token-2" {
		t.Errorf("retry used %q, want a fresh token", got)
	}
	if tokens, api := f.tokenCalls.Load(), f.apiCalls.Load(); tokens != 2 || api != 3 {
		t.Errorf("token requests = %d, API calls = %d; want 2 and 3", tokens, api)
	}
}

func TestSendRetriesWithIdenticalBodyOn401(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			respond(200, `{"codRespuesta":"98"}`)(w, r)
			return
		}
		respond(200, `{"numTicket":"t"}`)(w, r)
	}
	c := f.client()
	if _, err := c.Status(context.Background(), "t"); err != nil { // caches token-1
		t.Fatal(err)
	}
	f.mu.Lock()
	f.valid = map[string]bool{}
	f.bodies = nil
	f.mu.Unlock()

	if _, err := c.Send(context.Background(), testFileName, []byte("<x/>")); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if len(f.bodies) != 2 {
		t.Fatalf("API received %d requests, want the rejected one and its retry", len(f.bodies))
	}
	var payload struct{ Archivo struct{ ArcGreZip string } }
	if err := json.Unmarshal(f.bodies[1], &payload); err != nil || payload.Archivo.ArcGreZip == "" {
		t.Errorf("retried request lost its body: %v", err)
	}
	if !bytes.Equal(f.bodies[0], f.bodies[1]) {
		t.Error("retry must resend the identical body")
	}
}

func TestPersistent401IsNotRetriedForever(t *testing.T) {
	f := newFakeSUNAT(t)
	f.rejectAll = true
	f.apiHandler = respond(200, `{}`)
	_, err := f.client().Status(context.Background(), "t")
	var ge *gre.Error
	if !errors.As(err, &ge) || ge.HTTPStatus != 401 || ge.Code != "401" {
		t.Fatalf("error = %v, want *gre.Error with HTTP 401", err)
	}
	if tokens, api := f.tokenCalls.Load(), f.apiCalls.Load(); tokens != 2 || api != 2 {
		t.Errorf("token requests = %d, API calls = %d; want exactly one retry (2 and 2)", tokens, api)
	}
}

func TestHTTPErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		code    string
		message string
	}{
		{"gateway error", 500, `{"cod":"500","msg":"Error inesperado"}`, "500", "Error inesperado"},
		{"validation errors", 422, `{"cod":"422","msg":"Validacion","errors":[{"cod":"1033","msg":"El comprobante fue registrado previamente"}]}`,
			"422", "Validacion: 1033 El comprobante fue registrado previamente"},
		{"numeric cod", 400, `{"cod":400,"msg":"bad"}`, "400", "bad"},
		{"not json", 502, `<html>Bad Gateway</html>`, "", ""},
		{"secrets are scrubbed", 400, fmt.Sprintf(`{"cod":"400","msg":"bad %s and %s"}`, testClientSecret, testPassword),
			"400", "bad [redacted] and [redacted]"},
		{"long message is capped", 500, `{"cod":"500","msg":"` + strings.Repeat("x", 2000) + `"}`, "500", strings.Repeat("x", 500) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSUNAT(t)
			f.apiHandler = respond(tt.status, tt.body)
			_, err := f.client().Send(context.Background(), testFileName, []byte("<x/>"))
			var ge *gre.Error
			if !errors.As(err, &ge) {
				t.Fatalf("error = %v, want *gre.Error", err)
			}
			if ge.HTTPStatus != tt.status || ge.Code != tt.code || ge.Message != tt.message {
				t.Errorf("error = %+v, want status=%d code=%q message=%q", *ge, tt.status, tt.code, tt.message)
			}
			for _, leak := range []string{testClientSecret, testPassword, "Bad Gateway", "<html>"} {
				if strings.Contains(err.Error(), leak) {
					t.Errorf("error %q leaks %q", err, leak)
				}
			}
		})
	}
}

func TestErrorString(t *testing.T) {
	for _, tt := range []struct {
		err  gre.Error
		want string
	}{
		{gre.Error{Code: "1033", Message: "duplicado", HTTPStatus: 422}, "gre: 1033: duplicado"},
		{gre.Error{Code: "401", HTTPStatus: 401}, "gre: 401"},
		{gre.Error{Message: "solo mensaje"}, "gre: solo mensaje"},
		{gre.Error{HTTPStatus: 502}, "gre: HTTP 502"},
	} {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}

func TestTokenErrors(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{"codRespuesta":"98"}`)
	f.tokenStatus = http.StatusUnauthorized
	f.tokenBody = fmt.Sprintf(`{"error":"invalid_client","error_description":"bad credentials %s"}`, testClientSecret)
	_, err := f.client().Status(context.Background(), "t")
	var ge *gre.Error
	if !errors.As(err, &ge) {
		t.Fatalf("error = %v, want *gre.Error", err)
	}
	if ge.HTTPStatus != 401 || ge.Code != "invalid_client" || ge.Message != "bad credentials [redacted]" {
		t.Errorf("error = %+v", *ge)
	}
	if f.apiCalls.Load() != 0 {
		t.Error("API must not be called without a token")
	}
	// A failed token request is not cached: the next call asks again.
	f.tokenStatus = http.StatusOK
	if _, err := f.client().Status(context.Background(), "t"); err != nil {
		t.Errorf("recovery error: %v", err)
	}
}

func TestTokenResponseWithoutAccessToken(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{}`)
	f.tokenOK = `{"token_type":"Bearer","expires_in":3600}`
	if _, err := f.client().Status(context.Background(), "t"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMissingClientCredentials(t *testing.T) {
	f := newFakeSUNAT(t)
	c := gre.New(gre.Credentials{RUC: "20000000001", User: "U", Password: testPassword},
		gre.WithEndpoints(f.srv.URL+"/sec", f.srv.URL+"/api"))
	_, err := c.Send(context.Background(), testFileName, []byte("<x/>"))
	if err == nil || strings.Contains(err.Error(), testPassword) {
		t.Fatalf("error = %v", err)
	}
	if f.tokenCalls.Load() != 0 {
		t.Error("no request should be made without client credentials")
	}
}

func TestConcurrentCallsShareOneToken(t *testing.T) {
	f := newFakeSUNAT(t)
	f.tokenDelay = 50 * time.Millisecond
	f.apiHandler = respond(200, `{"codRespuesta":"98"}`)
	c := f.client()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Status(context.Background(), "t")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Status() error: %v", err)
		}
	}
	if n := f.tokenCalls.Load(); n != 1 {
		t.Errorf("token requests = %d, want 1", n)
	}
}

func TestContextCancellation(t *testing.T) {
	f := newFakeSUNAT(t)
	f.tokenDelay = 200 * time.Millisecond
	f.apiHandler = respond(200, `{}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.client().Status(ctx, "t"); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}

	// A caller waiting for another goroutine's token request honors its own context.
	c := f.client()
	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = c.Status(context.Background(), "t")
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if _, err := c.Status(ctx2, "t"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiting caller error = %v, want context.DeadlineExceeded", err)
	}
}

func TestWithHTTPClient(t *testing.T) {
	f := newFakeSUNAT(t)
	f.apiHandler = respond(200, `{"codRespuesta":"98"}`)
	used := false
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(r)
	})}
	if _, err := f.client(gre.WithHTTPClient(hc)).Status(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Error("custom HTTP client not used")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDefaultEndpoints(t *testing.T) {
	var urls []string
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		urls = append(urls, r.Method+" "+r.URL.String())
		if strings.Contains(r.URL.Host, "seguridad") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"a","expires_in":3600}`)), Header: http.Header{}}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"codRespuesta":"98"}`)), Header: http.Header{}}, nil
	})}
	if _, err := gre.New(testCreds, gre.WithHTTPClient(hc)).Status(context.Background(), "tk"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST https://api-seguridad.sunat.gob.pe/v1/clientessol/test-client-id/oauth2/token/",
		"GET https://api-cpe.sunat.gob.pe/v1/contribuyente/gem/comprobantes/envios/tk",
	}
	if strings.Join(urls, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests = %q, want %q", urls, want)
	}
}

func TestClientImplementsTransport(t *testing.T) {
	var _ gre.Transport = gre.New(testCreds)
}
