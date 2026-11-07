package transport

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// ConsultProduction is the billConsultService endpoint that serves getStatusCdr.
//
// SUNAT publishes no beta consult service (its list of web services has a beta
// billService only), so a SOAPClient built with BillBeta has no consult
// endpoint and GetStatusCdr returns ErrNoConsultEndpoint. Use WithConsultEndpoint
// to point it elsewhere, e.g. a test server. Consult needs the SOL credentials
// of the issuer, so production calls require real ones.
const ConsultProduction = "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService"

// ErrNoConsultEndpoint is returned by GetStatusCdr when the client has no
// billConsultService endpoint, as happens with the beta billService.
var ErrNoConsultEndpoint = errors.New("transport: no billConsultService endpoint configured")

// Ticket status codes returned by getStatus (the ticket flow).
const (
	TicketProcessed = "0"  // Processed successfully; Content holds the CDR.
	TicketInProcess = "98" // Still processing; no content yet.
	TicketErrors    = "99" // Processed with errors; Content may hold a rejecting CDR.
)

// Status is the answer of getStatus or getStatusCdr.
type Status struct {
	Code    string // statusCode as SUNAT sent it, e.g. "0", "98", "99", "0004"
	Message string // statusMessage; only the consult service sends one
	Content []byte // zipped CDR; empty when SUNAT sent none
}

// number returns Code as an integer so "98" and "0098" compare equal.
func (s Status) number() (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s.Code))
	return n, err == nil
}

// InProcess reports that SUNAT has not finished the ticket yet (code 98).
// The caller should poll GetStatus again later.
func (s Status) InProcess() bool {
	n, ok := s.number()
	return ok && n == 98
}

// Done reports that a ticket finished, with or without errors (code 0 or 99).
// It is meaningful for getStatus only.
func (s Status) Done() bool {
	n, ok := s.number()
	return ok && (n == 0 || n == 99)
}

// WithErrors reports a ticket processed with errors (code 99). Content may
// still carry a CDR explaining the rejection.
func (s Status) WithErrors() bool {
	n, ok := s.number()
	return ok && n == 99
}

// BatchTransport sends documents SUNAT processes asynchronously (Resumen
// diario RC, Comunicación de baja RA) and polls their ticket.
type BatchTransport interface {
	// SendSummary sends signedXML as fileName.zip with sendSummary and returns
	// the ticket.
	SendSummary(ctx context.Context, fileName string, signedXML []byte) (ticket string, err error)
	// GetStatus asks for the state of a ticket with getStatus.
	GetStatus(ctx context.Context, ticket string) (Status, error)
}

// ConsultTransport recovers the CDR of a document already sent.
type ConsultTransport interface {
	// GetStatusCdr asks billConsultService for the CDR of a comprobante.
	GetStatusCdr(ctx context.Context, ruc, docType, series string, number int64) (Status, error)
}

var (
	_ BatchTransport   = (*SOAPClient)(nil)
	_ ConsultTransport = (*SOAPClient)(nil)
)

// WithConsultEndpoint overrides the billConsultService endpoint used by
// GetStatusCdr. The default is ConsultProduction when the client targets
// BillProduction, and none otherwise.
func WithConsultEndpoint(endpoint string) Option {
	return func(s *SOAPClient) { s.consultEndpoint = endpoint }
}

func defaultConsultEndpoint(billEndpoint string) string {
	if billEndpoint == BillProduction {
		return ConsultProduction
	}
	return ""
}

// statusBody is the status element of getStatus and getStatusCdr responses.
type statusBody struct {
	Content    string `xml:"content"`
	StatusCode string `xml:"statusCode"`
	Message    string `xml:"statusMessage"`
}

func (b *statusBody) status(op string) (Status, error) {
	code := strings.TrimSpace(b.StatusCode)
	if code == "" {
		return Status{}, fmt.Errorf("transport: %s response has no statusCode", op)
	}
	st := Status{Code: code, Message: strings.TrimSpace(b.Message)}
	if raw := stripSpace(b.Content); raw != "" {
		content, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return Status{}, fmt.Errorf("transport: decode %s content: %w", op, err)
		}
		st.Content = content
	}
	return st, nil
}

// stripSpace removes whitespace, since SOAP stacks may wrap base64 lines.
func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// SendSummary zips signedXML as fileName.xml, sends it with sendSummary and
// returns the ticket to poll with GetStatus.
func (c *SOAPClient) SendSummary(ctx context.Context, fileName string, signedXML []byte) (string, error) {
	zipped, err := Zip(fileName+".xml", signedXML)
	if err != nil {
		return "", err
	}
	body := `<ser:sendSummary><fileName>` + escape(fileName+".zip") + `</fileName><contentFile>` +
		base64.StdEncoding.EncodeToString(zipped) + `</contentFile></ser:sendSummary>`
	env, err := c.call(ctx, "urn:sendSummary", body)
	if err != nil {
		return "", err
	}
	if env.Body.SendSummary == nil || strings.TrimSpace(env.Body.SendSummary.Ticket) == "" {
		return "", errors.New("transport: sendSummary response has no ticket")
	}
	return strings.TrimSpace(env.Body.SendSummary.Ticket), nil
}

// GetStatus asks SUNAT for the state of a ticket returned by SendSummary.
// Status.InProcess means poll again; Status.Done means Content holds the CDR.
// SUNAT-side failures such as an unknown ticket arrive as *Error.
func (c *SOAPClient) GetStatus(ctx context.Context, ticket string) (Status, error) {
	if strings.TrimSpace(ticket) == "" {
		return Status{}, errors.New("transport: getStatus requires a ticket")
	}
	body := `<ser:getStatus><ticket>` + escape(ticket) + `</ticket></ser:getStatus>`
	env, err := c.call(ctx, "urn:getStatus", body)
	if err != nil {
		return Status{}, err
	}
	if env.Body.GetStatus == nil {
		return Status{}, errors.New("transport: getStatus response has no status")
	}
	return env.Body.GetStatus.status("getStatus")
}

// GetStatusCdr asks billConsultService for the CDR of a comprobante. SUNAT
// documents it for Factura (01) and notes (07, 08). Status.Content holds the
// zipped CDR when SUNAT has one.
func (c *SOAPClient) GetStatusCdr(ctx context.Context, ruc, docType, series string, number int64) (Status, error) {
	if c.consultEndpoint == "" {
		return Status{}, ErrNoConsultEndpoint
	}
	// numeroComprobante is xsd:int in the SUNAT schema.
	if number < 1 || number > math.MaxInt32 {
		return Status{}, fmt.Errorf("transport: getStatusCdr number %d out of range", number)
	}
	body := `<ser:getStatusCdr>` +
		`<rucComprobante>` + escape(ruc) + `</rucComprobante>` +
		`<tipoComprobante>` + escape(docType) + `</tipoComprobante>` +
		`<serieComprobante>` + escape(series) + `</serieComprobante>` +
		`<numeroComprobante>` + strconv.FormatInt(number, 10) + `</numeroComprobante>` +
		`</ser:getStatusCdr>`
	env, err := c.callEndpoint(ctx, c.consultEndpoint, "urn:getStatusCdr", body)
	if err != nil {
		return Status{}, err
	}
	if env.Body.GetStatusCdr == nil {
		return Status{}, errors.New("transport: getStatusCdr response has no statusCdr")
	}
	return env.Body.GetStatusCdr.status("getStatusCdr")
}
