package sunat

import (
	"context"
	"errors"
	"fmt"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/transport"
	"github.com/villano14/sunat-go/ubl"
)

// ErrBatchUnsupported is returned when the Client's transport cannot send
// summaries or poll tickets (it does not implement transport.BatchTransport).
var ErrBatchUnsupported = errors.New("sunat: transport does not support sendSummary/getStatus")

// ErrConsultUnsupported is returned when the Client's transport cannot
// recover CDRs (it does not implement transport.ConsultTransport).
var ErrConsultUnsupported = errors.New("sunat: transport does not support getStatusCdr")

// BatchResult holds the signed XML of a Resumen diario or Comunicación de baja
// and the ticket SUNAT issued for it.
type BatchResult struct {
	XML    []byte // Signed document; available even when sending fails.
	Ticket string // Poll it with Client.Status.
}

// BatchState is the outcome of a ticket.
type BatchState string

const (
	BatchInProcess BatchState = "in_process" // SUNAT has not finished; poll again later
	BatchAccepted  BatchState = "accepted"   // processed and the CDR accepts the document
	BatchRejected  BatchState = "rejected"   // processed with errors or a rejecting CDR
)

// StatusResult is the parsed answer of getStatus for a ticket.
type StatusResult struct {
	State       BatchState
	StatusCode  string       // statusCode as SUNAT sent it: "0", "98" or "99"
	Code        string       // SUNAT response code: the CDR's, or StatusCode when there is no CDR
	Description string       // SUNAT response description
	CDR         cdr.Response // Zero unless SUNAT sent a CDR
	CDRZip      []byte       // Zipped CDR as received, to be stored as proof
}

// InProcess reports that SUNAT has not finished processing the ticket.
func (r StatusResult) InProcess() bool { return r.State == BatchInProcess }

// Accepted reports that SUNAT processed the ticket and accepted the document.
func (r StatusResult) Accepted() bool { return r.State == BatchAccepted }

// Rejected reports that SUNAT processed the ticket with errors.
func (r StatusResult) Rejected() bool { return r.State == BatchRejected }

// QueryResult is the parsed answer of getStatusCdr.
type QueryResult struct {
	StatusCode string       // statusCode as SUNAT sent it, e.g. "0004"
	Message    string       // statusMessage
	CDR        cdr.Response // Zero unless SUNAT sent a CDR
	CDRZip     []byte       // Zipped CDR as received, to be stored as proof
}

// Found reports that SUNAT returned a CDR for the comprobante.
func (r QueryResult) Found() bool { return len(r.CDRZip) > 0 }

// Accepted reports that SUNAT returned a CDR and it accepts the comprobante.
func (r QueryResult) Accepted() bool { return r.Found() && r.CDR.Accepted() }

// BuildSummary renders and signs s without sending it or performing I/O.
func (c *Client) BuildSummary(s model.Summary) ([]byte, error) {
	xml, err := ubl.Summary(s)
	if err != nil {
		return nil, fmt.Errorf("sunat: build summary: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign summary: %w", err)
	}
	return signed, nil
}

// BuildVoided renders and signs v without sending it or performing I/O.
func (c *Client) BuildVoided(v model.Voided) ([]byte, error) {
	xml, err := ubl.Voided(v)
	if err != nil {
		return nil, fmt.Errorf("sunat: build voided: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign voided: %w", err)
	}
	return signed, nil
}

// SendSummary builds and signs s, sends it with sendSummary and returns the
// ticket. Once signing succeeds, BatchResult.XML is returned even on error.
// Poll the ticket with Status. The transport must implement
// transport.BatchTransport, otherwise the error wraps ErrBatchUnsupported.
func (c *Client) SendSummary(ctx context.Context, s model.Summary) (BatchResult, error) {
	xml, err := c.BuildSummary(s)
	if err != nil {
		return BatchResult{}, err
	}
	return c.sendBatch(ctx, "summary", s.FileName(), xml)
}

// SendVoided builds and signs v, sends it with sendSummary and returns the
// ticket. Once signing succeeds, BatchResult.XML is returned even on error.
// Poll the ticket with Status. The transport must implement
// transport.BatchTransport, otherwise the error wraps ErrBatchUnsupported.
func (c *Client) SendVoided(ctx context.Context, v model.Voided) (BatchResult, error) {
	xml, err := c.BuildVoided(v)
	if err != nil {
		return BatchResult{}, err
	}
	return c.sendBatch(ctx, "voided", v.FileName(), xml)
}

func (c *Client) sendBatch(ctx context.Context, kind, fileName string, xml []byte) (BatchResult, error) {
	result := BatchResult{XML: xml}
	bt, ok := c.tr.(transport.BatchTransport)
	if !ok {
		return result, fmt.Errorf("sunat: send %s: %w", kind, ErrBatchUnsupported)
	}
	ticket, err := bt.SendSummary(ctx, fileName, xml)
	if err != nil {
		return result, fmt.Errorf("sunat: send %s: %w", kind, err)
	}
	if ticket == "" {
		return result, fmt.Errorf("sunat: send %s: SUNAT returned an empty ticket", kind)
	}
	result.Ticket = ticket
	return result, nil
}

// Status asks SUNAT for the state of a ticket from SendSummary or SendVoided.
//
// While SUNAT is working, the result is InProcess (code 98) and the caller
// should poll again later. Once finished, the CDR is parsed: code 0 with an
// accepting CDR is Accepted; code 99, or a CDR that does not accept, is
// Rejected. A rejection is a result, not an error; transport errors such as an
// unknown ticket keep their type. The transport must implement
// transport.BatchTransport, otherwise the error wraps ErrBatchUnsupported.
func (c *Client) Status(ctx context.Context, ticket string) (StatusResult, error) {
	bt, ok := c.tr.(transport.BatchTransport)
	if !ok {
		return StatusResult{}, fmt.Errorf("sunat: status: %w", ErrBatchUnsupported)
	}
	st, err := bt.GetStatus(ctx, ticket)
	if err != nil {
		return StatusResult{}, fmt.Errorf("sunat: status: %w", err)
	}
	result := StatusResult{StatusCode: st.Code, Code: st.Code}
	switch {
	case st.InProcess():
		result.State = BatchInProcess
		result.Description = "ticket still in process"
		return result, nil
	case !st.Done():
		return result, fmt.Errorf("sunat: status: unexpected statusCode %q", st.Code)
	}

	result.State = BatchRejected
	if len(st.Content) == 0 {
		if !st.WithErrors() {
			return result, errors.New("sunat: status: SUNAT reported success without a CDR")
		}
		result.Description = "processed with errors; SUNAT returned no CDR"
		return result, nil
	}
	result.CDRZip = st.Content
	result.CDR, err = cdr.ParseZip(st.Content)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	result.Code, result.Description = result.CDR.Code, result.CDR.Description
	if !st.WithErrors() && result.CDR.Accepted() {
		result.State = BatchAccepted
	}
	return result, nil
}

// QueryCDR recovers the CDR of a comprobante already sent, using
// getStatusCdr on billConsultService. SUNAT documents it for Factura (01) and
// notes (07, 08); boletas are recovered through the ticket of their summary.
//
// A comprobante SUNAT does not know is a result without CDR (Found is false;
// see StatusCode and Message), not an error. SUNAT publishes no beta consult
// service, so transport.SOAPClient returns transport.ErrNoConsultEndpoint
// unless it targets production or has transport.WithConsultEndpoint. The
// transport must implement transport.ConsultTransport, otherwise the error
// wraps ErrConsultUnsupported.
func (c *Client) QueryCDR(ctx context.Context, ruc string, docType catalog.DocumentType, series string, number int64) (QueryResult, error) {
	ct, ok := c.tr.(transport.ConsultTransport)
	if !ok {
		return QueryResult{}, fmt.Errorf("sunat: query CDR: %w", ErrConsultUnsupported)
	}
	st, err := ct.GetStatusCdr(ctx, ruc, string(docType), series, number)
	if err != nil {
		return QueryResult{}, fmt.Errorf("sunat: query CDR: %w", err)
	}
	result := QueryResult{StatusCode: st.Code, Message: st.Message}
	if len(st.Content) == 0 {
		return result, nil
	}
	result.CDRZip = st.Content
	result.CDR, err = cdr.ParseZip(st.Content)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	return result, nil
}
