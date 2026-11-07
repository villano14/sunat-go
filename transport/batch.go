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

// ConsultProduction is the billConsultService endpoint for document status
// (getStatus) and CDR recovery (getStatusCdr), not ticket polling.
//
// SUNAT publishes no beta consult service (its list of web services has a beta
// billService only), so a SOAPClient built with BillBeta has no consult
// endpoint and GetStatusCdr returns ErrNoConsultEndpoint. Use WithConsultEndpoint
// to point it elsewhere, e.g. a test server. Consult needs the SOL credentials
// of the issuer, so production calls require real ones.
const ConsultProduction = "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService"

// ErrNoConsultEndpoint is returned by GetStatusCdr or GetStatusCpe when the client has no
// billConsultService endpoint, as happens with the beta billService.
var ErrNoConsultEndpoint = errors.New("transport: no billConsultService endpoint configured")

// Ticket status codes returned by getStatus (the ticket flow).
const (
	TicketProcessed = "0"  // Processed successfully; Content holds the response archive.
	TicketInProcess = "98" // Still processing; no content yet.
	TicketErrors    = "99" // Processed with errors; Content may hold a rejecting CDR.
)

// Status is the answer of getStatus or getStatusCdr.
type Status struct {
	Code    string // statusCode as SUNAT sent it, e.g. "0", "98", "99", "0004"
	Message string // statusMessage; only the consult service sends one
	Content []byte // raw ZIP: CDR, or per-document CDRs/report for packs; empty when absent
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

// PackTransport is an optional sendPack capability, independent of
// BatchTransport. Callers prepare the ZIP of signed XML documents themselves.
// SOAPClient.GetStatus polls the returned ticket without parsing its archive.
type PackTransport interface {
	SendPack(ctx context.Context, fileName string, zipContent []byte) (ticket string, err error)
}

// ConsultTransport recovers the CDR of a document already sent.
type ConsultTransport interface {
	// GetStatusCdr asks billConsultService for the CDR of a comprobante.
	GetStatusCdr(ctx context.Context, ruc, docType, series string, number int64) (Status, error)
}

// CPEStatus is the status-only answer from billConsultService. Codes retain
// leading zeros: 0001 accepted, 0002 rejected, 0003 voided, 0011 absent.
// Unknown codes are preserved. This answer is not a CDR or a ticket status.
type CPEStatus struct {
	Code    string
	Message string
}

// CPEStatusTransport is an optional capability for document status queries.
// It does not extend ConsultTransport, so existing implementations remain valid.
type CPEStatusTransport interface {
	GetStatusCpe(ctx context.Context, ruc, docType, series string, number int64) (CPEStatus, error)
}

var (
	_ BatchTransport     = (*SOAPClient)(nil)
	_ PackTransport      = (*SOAPClient)(nil)
	_ ConsultTransport   = (*SOAPClient)(nil)
	_ CPEStatusTransport = (*SOAPClient)(nil)
)

// WithConsultEndpoint overrides the billConsultService endpoint used by
// GetStatusCdr and GetStatusCpe. The default is ConsultProduction when the client targets
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

// SendPack sends a caller-prepared ZIP of signed XML documents with sendPack
// and returns the ticket to poll with GetStatus. fileName is the complete ZIP
// filename (including .zip); zipContent is base64-encoded unchanged, not zipped
// again. This method does not build, sign or validate the archive's documents.
func (c *SOAPClient) SendPack(ctx context.Context, fileName string, zipContent []byte) (string, error) {
	body := `<ser:sendPack><fileName>` + escape(fileName) + `</fileName><contentFile>` +
		base64.StdEncoding.EncodeToString(zipContent) + `</contentFile></ser:sendPack>`
	env, err := c.call(ctx, "urn:sendPack", body)
	if err != nil {
		return "", err
	}
	if env.Body.SendPack == nil || strings.TrimSpace(env.Body.SendPack.Ticket) == "" {
		return "", errors.New("transport: sendPack response has no ticket")
	}
	return strings.TrimSpace(env.Body.SendPack.Ticket), nil
}

// GetStatus asks SUNAT for the state of a ticket returned by SendSummary or
// SendPack. Status.InProcess means poll again. Content is the raw response ZIP;
// a pack may contain multiple per-document CDRs and a report. Done alone does
// not establish that every document was accepted, or that content is present.
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
	env, err := c.consultQuery(ctx, "getStatusCdr", ruc, docType, series, number)
	if err != nil {
		return Status{}, err
	}
	if env.Body.GetStatusCdr == nil {
		return Status{}, errors.New("transport: getStatusCdr response has no statusCdr")
	}
	return env.Body.GetStatusCdr.status("getStatusCdr")
}

// GetStatusCpe asks billConsultService for a document's status, without CDR
// recovery. SUNAT documents Factura (01) and F-series notes (07, 08). It is
// unrelated to GetStatus(ticket) on billService. Optional response content is
// deliberately ignored: it is not evidence of acceptance or a CDR to decode.
func (c *SOAPClient) GetStatusCpe(ctx context.Context, ruc, docType, series string, number int64) (CPEStatus, error) {
	env, err := c.consultQuery(ctx, "getStatus", ruc, docType, series, number)
	if err != nil {
		return CPEStatus{}, err
	}
	b := env.Body.GetStatus
	if b == nil {
		return CPEStatus{}, errors.New("transport: getStatusCpe response has no status")
	}
	code := strings.TrimSpace(b.StatusCode)
	if code == "" {
		return CPEStatus{}, errors.New("transport: getStatusCpe response has no statusCode")
	}
	return CPEStatus{Code: code, Message: strings.TrimSpace(b.Message)}, nil
}

// consultQuery keeps the existing CDR query's validation and wire format in
// one place for both consult operations. op is an internal SOAP operation name.
func (c *SOAPClient) consultQuery(ctx context.Context, op, ruc, docType, series string, number int64) (*envelope, error) {
	if c.consultEndpoint == "" {
		return nil, ErrNoConsultEndpoint
	}
	// numeroComprobante is xsd:int in the SUNAT schema.
	if number < 1 || number > math.MaxInt32 {
		return nil, fmt.Errorf("transport: %s number %d out of range", op, number)
	}
	body := `<ser:` + op + `>` +
		`<rucComprobante>` + escape(ruc) + `</rucComprobante>` +
		`<tipoComprobante>` + escape(docType) + `</tipoComprobante>` +
		`<serieComprobante>` + escape(series) + `</serieComprobante>` +
		`<numeroComprobante>` + strconv.FormatInt(number, 10) + `</numeroComprobante>` +
		`</ser:` + op + `>`
	return c.callEndpoint(ctx, c.consultEndpoint, "urn:"+op, body)
}
