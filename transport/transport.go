// Package transport sends documents to SUNAT SOAP web services.
package transport

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SUNAT billService endpoints.
const (
	BillBeta       = "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService"
	BillProduction = "https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService"
)

// maxResponse caps the bytes read from SUNAT to protect against runaway bodies.
const maxResponse = 32 << 20

// Credentials are the issuer's SOL credentials. The WS-Security username is
// RUC followed by User, e.g. "20000000001MODDATOS".
type Credentials struct {
	RUC      string
	User     string
	Password string
}

// Transport sends a signed document and returns the zipped CDR.
type Transport interface {
	SendBill(ctx context.Context, fileName string, signedXML []byte) ([]byte, error)
}

// Error is a SOAP fault returned by SUNAT. Code holds the SUNAT error code
// (e.g. "0151") when the fault carries one.
type Error struct {
	Code      string
	Message   string
	FaultCode string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return "sunat: " + e.Code + ": " + e.Message
	}
	return "sunat: soap fault " + e.FaultCode + ": " + e.Message
}

// SOAPClient is a Transport for the SUNAT billService.
type SOAPClient struct {
	endpoint string
	creds    Credentials
	http     *http.Client
}

// Option configures a SOAPClient.
type Option func(*SOAPClient)

// WithHTTPClient replaces the default HTTP client (60s timeout).
func WithHTTPClient(c *http.Client) Option { return func(s *SOAPClient) { s.http = c } }

// New returns a client for the given endpoint, e.g. BillBeta.
func New(endpoint string, creds Credentials, opts ...Option) *SOAPClient {
	c := &SOAPClient{endpoint: endpoint, creds: creds, http: &http.Client{Timeout: 60 * time.Second}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// SendBill zips signedXML as fileName.xml, sends it with sendBill and returns
// the zipped CDR (applicationResponse).
func (c *SOAPClient) SendBill(ctx context.Context, fileName string, signedXML []byte) ([]byte, error) {
	zipped, err := Zip(fileName+".xml", signedXML)
	if err != nil {
		return nil, err
	}
	body := `<ser:sendBill><fileName>` + escape(fileName+".zip") + `</fileName><contentFile>` +
		base64.StdEncoding.EncodeToString(zipped) + `</contentFile></ser:sendBill>`
	env, err := c.call(ctx, "urn:sendBill", body)
	if err != nil {
		return nil, err
	}
	if env.Body.SendBill == nil {
		return nil, fmt.Errorf("transport: sendBill response has no applicationResponse")
	}
	cdr, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env.Body.SendBill.ApplicationResponse))
	if err != nil {
		return nil, fmt.Errorf("transport: decode applicationResponse: %w", err)
	}
	return cdr, nil
}

type envelope struct {
	Body struct {
		Fault *struct {
			Code   string `xml:"faultcode"`
			String string `xml:"faultstring"`
		} `xml:"Fault"`
		SendBill *struct {
			ApplicationResponse string `xml:"applicationResponse"`
		} `xml:"sendBillResponse"`
	} `xml:"Body"`
}

func (c *SOAPClient) call(ctx context.Context, action, body string) (*envelope, error) {
	payload := `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ser="http://service.sunat.gob.pe" xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">` +
		`<soapenv:Header><wsse:Security><wsse:UsernameToken>` +
		`<wsse:Username>` + escape(c.creds.RUC+c.creds.User) + `</wsse:Username>` +
		`<wsse:Password>` + escape(c.creds.Password) + `</wsse:Password>` +
		`</wsse:UsernameToken></wsse:Security></soapenv:Header>` +
		`<soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("transport: build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"`+action+`"`)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("transport: read response: %w", err)
	}

	var env envelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("transport: HTTP %d: unexpected response: %s", res.StatusCode, snippet(raw))
	}
	if f := env.Body.Fault; f != nil {
		return nil, &Error{Code: faultCode(f.Code, f.String), Message: strings.TrimSpace(f.String), FaultCode: f.Code}
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transport: HTTP %d: %s", res.StatusCode, snippet(raw))
	}
	return &env, nil
}

// faultCode extracts the SUNAT code from "soap-env:Client.0151" or, failing
// that, from a numeric fault string.
func faultCode(code, str string) string {
	if i := strings.LastIndexByte(code, '.'); i >= 0 && isDigits(code[i+1:]) {
		return code[i+1:]
	}
	if s := strings.TrimSpace(str); isDigits(s) {
		return s
	}
	return ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Zip packs one file into a ZIP archive, the format SUNAT expects.
func Zip(name string, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		return nil, fmt.Errorf("transport: zip: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("transport: zip: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("transport: zip: %w", err)
	}
	return buf.Bytes(), nil
}

func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func snippet(b []byte) string {
	const n = 200
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
