// Package gre sends Guías de Remisión Electrónicas (GRE, types 09 and 31) to
// SUNAT's REST API.
//
// Unlike invoices, GRE does not use SOAP. The flow has three steps:
//
//  1. An OAuth2 password-grant token is requested from SUNAT's security
//     service, using the client_id and client_secret generated in the SOL
//     menu ("Credenciales de API SUNAT") plus the SOL user.
//  2. The signed XML, zipped, is POSTed as JSON and SUNAT answers with a ticket.
//  3. The ticket is polled until SUNAT returns the zipped CDR.
//
// Client caches the token in memory, renews it before it expires, and retries
// a request once when SUNAT answers 401.
//
// # Environments
//
// SUNAT does not publish a beta host for GRE: production credentials are the
// only ones its servers accept. The beta environment Greenter uses is a
// third-party homologation server run by Nubefact, which speaks the same
// protocol (BetaBase, with its own public test client_id and client_secret).
// Point a Client at it with WithEndpoints; never send real documents or SOL
// credentials there.
package gre

import (
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
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/villano14/sunat-go/transport"
)

// Service base URLs, without trailing slash.
const (
	SecurityProduction = "https://api-seguridad.sunat.gob.pe/v1" // OAuth2 token service
	APIProduction      = "https://api-cpe.sunat.gob.pe/v1"       // Comprobantes service
	// BetaBase serves both services on Nubefact's homologation server. It is
	// not operated by SUNAT.
	BetaBase = "https://gre-test.nubefact.com/v1"
)

// Scope is the fixed OAuth2 scope of the token request.
const Scope = "https://api-cpe.sunat.gob.pe"

// Status codes SUNAT reports in codRespuesta.
const (
	CodeAccepted  = "0"
	CodeInProcess = "98"
	CodeRejected  = "99"
)

const (
	// maxResponse caps the bytes read from SUNAT to protect against runaway bodies.
	maxResponse = 32 << 20
	// maxMessage caps the SUNAT-provided text kept in an Error.
	maxMessage = 500
	// defaultTokenTTL applies when the token response omits expires_in.
	defaultTokenTTL = time.Hour
)

// Credentials identify the issuer. ClientID and ClientSecret come from the SOL
// menu; the OAuth2 username is RUC followed by User, e.g. "20000000001MODDATOS".
type Credentials struct {
	ClientID     string
	ClientSecret string
	RUC          string
	User         string
	Password     string
}

// Ticket acknowledges a received GRE. Number is the only handle on the
// outcome: store it before doing anything else.
type Ticket struct {
	Number     string
	ReceivedAt time.Time // Zero when SUNAT omits it or sends an unknown format
}

// Status is SUNAT's answer for a ticket.
type Status struct {
	Code         string // CodeAccepted, CodeInProcess or CodeRejected
	CDRZip       []byte // Zipped CDR, when SUNAT generated one
	CDRGenerated bool   // indCdrGenerado
	Error        *Error // SUNAT's error when Code is CodeRejected
}

// Accepted reports whether SUNAT processed the guide successfully.
func (s Status) Accepted() bool { return s.Code == CodeAccepted }

// InProcess reports whether SUNAT is still processing the ticket: ask again later.
func (s Status) InProcess() bool { return s.Code == CodeInProcess }

// Rejected reports whether SUNAT processed the ticket with an error.
func (s Status) Rejected() bool { return s.Code == CodeRejected }

// Transport sends a signed GRE and queries the outcome. Client implements it
// over HTTP; tests inject fakes so they never reach SUNAT.
type Transport interface {
	// Send zips signedXML as fileName.xml, uploads it and returns the ticket.
	Send(ctx context.Context, fileName string, signedXML []byte) (Ticket, error)
	// Status queries a ticket. A rejected guide (code 99) is a Status, not an
	// error; errors are transport, HTTP or protocol failures.
	Status(ctx context.Context, ticket string) (Status, error)
}

// Error is a failure reported by SUNAT or its gateway. Code is the SUNAT
// error code (cod, numError, or the OAuth2 error), Message its description
// and HTTPStatus the HTTP status of the answer. It never contains raw
// response bodies or credentials.
type Error struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *Error) Error() string {
	switch {
	case e.Code != "" && e.Message != "":
		return "gre: " + e.Code + ": " + e.Message
	case e.Code != "":
		return "gre: " + e.Code
	case e.Message != "":
		return "gre: " + e.Message
	}
	return "gre: HTTP " + strconv.Itoa(e.HTTPStatus)
}

// Client is a Transport for SUNAT's GRE REST API.
type Client struct {
	creds       Credentials
	securityURL string
	apiURL      string
	http        *http.Client
	now         func() time.Time
	tokens      *tokenSource
}

var _ Transport = (*Client)(nil)

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client (60s timeout).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithEndpoints overrides the base URLs of the token service and the
// comprobantes service, e.g. a test server or BetaBase for both.
func WithEndpoints(securityURL, apiURL string) Option {
	return func(c *Client) {
		c.securityURL = strings.TrimRight(securityURL, "/")
		c.apiURL = strings.TrimRight(apiURL, "/")
	}
}

// WithClock replaces time.Now, which decides when a cached token expires.
func WithClock(now func() time.Time) Option { return func(c *Client) { c.now = now } }

// New returns a Client for production; use WithEndpoints for anything else.
// It makes no I/O calls: the token is requested on first use.
func New(creds Credentials, opts ...Option) *Client {
	c := &Client{
		creds:       creds,
		securityURL: SecurityProduction,
		apiURL:      APIProduction,
		http:        &http.Client{Timeout: 60 * time.Second},
		now:         time.Now,
	}
	for _, o := range opts {
		o(c)
	}
	c.tokens = newTokenSource(c.fetchToken, func() time.Time { return c.now() })
	return c
}

// Send zips signedXML as fileName.xml and uploads it, returning SUNAT's ticket.
// fileName has no extension: RUC-TT-SERIES-NUMBER, e.g. "20000000001-09-T001-1".
func (c *Client) Send(ctx context.Context, fileName string, signedXML []byte) (Ticket, error) {
	zipped, err := transport.Zip(fileName+".xml", signedXML)
	if err != nil {
		return Ticket{}, err
	}
	sum := sha256.Sum256(zipped)
	body, err := json.Marshal(sendRequest{Archivo: sendArchivo{
		NomArchivo: fileName + ".zip",
		ArcGreZip:  base64.StdEncoding.EncodeToString(zipped),
		HashZip:    hex.EncodeToString(sum[:]),
	}})
	if err != nil {
		return Ticket{}, fmt.Errorf("gre: encode request: %w", err)
	}
	var res sendResponse
	endpoint := c.apiURL + "/contribuyente/gem/comprobantes/" + url.PathEscape(fileName)
	if err := c.call(ctx, http.MethodPost, endpoint, body, &res); err != nil {
		return Ticket{}, err
	}
	if res.NumTicket == "" {
		return Ticket{}, errors.New("gre: send response has no numTicket")
	}
	return Ticket{Number: string(res.NumTicket), ReceivedAt: parseTime(string(res.FecRecepcion))}, nil
}

// Status queries the outcome of ticket.
func (c *Client) Status(ctx context.Context, ticket string) (Status, error) {
	if ticket == "" {
		return Status{}, errors.New("gre: ticket is required")
	}
	var res statusResponse
	endpoint := c.apiURL + "/contribuyente/gem/comprobantes/envios/" + url.PathEscape(ticket)
	if err := c.call(ctx, http.MethodGet, endpoint, nil, &res); err != nil {
		return Status{}, err
	}
	st := Status{Code: string(res.CodRespuesta), CDRGenerated: string(res.IndCdrGenerado) == "1"}
	switch st.Code {
	case CodeAccepted, CodeInProcess, CodeRejected:
	default:
		return Status{}, &Error{Code: st.Code, Message: "unexpected codRespuesta", HTTPStatus: http.StatusOK}
	}
	if res.ArcCdr != "" {
		cdr, err := base64.StdEncoding.DecodeString(strings.TrimSpace(res.ArcCdr))
		if err != nil {
			return Status{}, fmt.Errorf("gre: decode arcCdr: %w", err)
		}
		st.CDRZip = cdr
	}
	if res.Error != nil && (res.Error.NumError != "" || res.Error.DesError != "") {
		st.Error = &Error{
			Code:       string(res.Error.NumError),
			Message:    c.clean(string(res.Error.DesError)),
			HTTPStatus: http.StatusOK,
		}
	}
	return st, nil
}

type sendRequest struct {
	Archivo sendArchivo `json:"archivo"`
}

type sendArchivo struct {
	NomArchivo string `json:"nomArchivo"`
	ArcGreZip  string `json:"arcGreZip"`
	HashZip    string `json:"hashZip"`
}

type sendResponse struct {
	NumTicket    flexString `json:"numTicket"`
	FecRecepcion flexString `json:"fecRecepcion"`
}

type statusResponse struct {
	CodRespuesta   flexString `json:"codRespuesta"`
	ArcCdr         string     `json:"arcCdr"`
	IndCdrGenerado flexString `json:"indCdrGenerado"`
	Error          *struct {
		NumError flexString `json:"numError"`
		DesError flexString `json:"desError"`
	} `json:"error"`
}

// errorBody covers SUNAT's error shapes: the gateway's {cod, msg, errors[]}
// and OAuth2's {error, error_description}.
type errorBody struct {
	Cod              flexString  `json:"cod"`
	Msg              flexString  `json:"msg"`
	Errors           []errorBody `json:"errors"`
	Error            flexString  `json:"error"`
	ErrorDescription flexString  `json:"error_description"`
}

// flexString decodes a JSON string or number as a string: SUNAT is not
// consistent about quoting codes.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// call performs an authenticated request and decodes a 2xx JSON answer into
// out. A 401 invalidates the token and the request is repeated once.
func (c *Client) call(ctx context.Context, method, endpoint string, body []byte, out any) error {
	rejected := ""
	for attempt := 0; ; attempt++ {
		token, err := c.tokens.get(ctx, rejected)
		if err != nil {
			return err
		}
		status, raw, err := c.roundTrip(ctx, method, endpoint, body, func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+token)
		})
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized && attempt == 0 {
			rejected = token
			continue
		}
		if status < 200 || status > 299 {
			return c.httpError(status, raw)
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("gre: decode response: %w", err)
		}
		return nil
	}
}

// roundTrip sends one request and returns the status and the capped body.
func (c *Client) roundTrip(ctx context.Context, method, endpoint string, body []byte, decorate func(*http.Request)) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rd)
	if err != nil {
		return 0, nil, fmt.Errorf("gre: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	decorate(req)
	res, err := c.http.Do(req)
	if err != nil {
		// url.Error carries the URL, which holds the client id but no secrets.
		return 0, nil, fmt.Errorf("gre: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return 0, nil, fmt.Errorf("gre: read response: %w", err)
	}
	return res.StatusCode, raw, nil
}

// fetchToken requests an OAuth2 password-grant token.
func (c *Client) fetchToken(ctx context.Context) (string, time.Duration, error) {
	if c.creds.ClientID == "" || c.creds.ClientSecret == "" {
		return "", 0, errors.New("gre: client id and client secret are required")
	}
	form := url.Values{
		"grant_type":    {"password"},
		"scope":         {Scope},
		"client_id":     {c.creds.ClientID},
		"client_secret": {c.creds.ClientSecret},
		"username":      {c.creds.RUC + c.creds.User},
		"password":      {c.creds.Password},
	}
	endpoint := c.securityURL + "/clientessol/" + url.PathEscape(c.creds.ClientID) + "/oauth2/token/"
	status, raw, err := c.roundTrip(ctx, http.MethodPost, endpoint, []byte(form.Encode()), func(req *http.Request) {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	if err != nil {
		return "", 0, err
	}
	if status < 200 || status > 299 {
		return "", 0, c.httpError(status, raw)
	}
	var tok struct {
		AccessToken string     `json:"access_token"`
		ExpiresIn   flexString `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return "", 0, fmt.Errorf("gre: decode token response: %w", err)
	}
	ttl := defaultTokenTTL
	if secs, err := strconv.Atoi(string(tok.ExpiresIn)); err == nil && secs > 0 {
		ttl = time.Duration(secs) * time.Second
	}
	return tok.AccessToken, ttl, nil
}

// httpError builds an Error from a non-2xx answer. Only the parsed SUNAT
// code and description are kept, never the body itself, and credentials are
// scrubbed in case a gateway echoes them.
func (c *Client) httpError(status int, raw []byte) *Error {
	e := &Error{HTTPStatus: status}
	var b errorBody
	if json.Unmarshal(raw, &b) != nil {
		return e
	}
	e.Code = c.clean(firstNonEmpty(string(b.Cod), string(b.Error)))
	e.Message = c.clean(firstNonEmpty(string(b.Msg), string(b.ErrorDescription)))
	var details []string
	for _, d := range b.Errors {
		if s := strings.TrimSpace(strings.Join([]string{string(d.Cod), string(d.Msg)}, " ")); s != "" {
			details = append(details, s)
		}
	}
	if len(details) > 0 {
		if e.Message != "" {
			e.Message += ": "
		}
		e.Message += c.clean(strings.Join(details, "; "))
	}
	return e
}

// clean makes SUNAT-provided text safe to keep: credentials removed, length capped.
func (c *Client) clean(s string) string {
	s = strings.TrimSpace(s)
	for _, secret := range []string{c.creds.ClientSecret, c.creds.Password} {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	if utf8.RuneCountInString(s) > maxMessage {
		s = string([]rune(s)[:maxMessage]) + "..."
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseTime reads fecRecepcion, tolerating the layouts SUNAT is known to use.
func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
