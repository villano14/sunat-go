package sunat

import (
	"context"
	"fmt"

	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/gre"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/sign"
	"github.com/villano14/sunat-go/ubl"
)

// Guías de Remisión Electrónicas (09 and 31) do not use the SOAP billService:
// SUNAT receives them through a REST API with OAuth2, and answers with a
// ticket whose CDR must be fetched afterwards. A Client holds a SOAP
// transport, so GRE has its own small facade, GREClient, which shares the
// signer:
//
//	signer := sign.New(cert)
//	invoices := sunat.New(signer, transport.New(transport.BillBeta, creds))
//	guides := sunat.NewGRE(signer, gre.New(greCreds))
//
//	sent, err := guides.SendDespatch(ctx, guide) // store sent.XML and sent.Ticket
//	...
//	res, err := guides.DespatchStatus(ctx, sent.Ticket.Number) // poll while res.Status.InProcess()

// BuildDespatch renders and signs d (type 09 or 31) without sending it or
// performing I/O.
func (c *Client) BuildDespatch(d model.Despatch) ([]byte, error) {
	return buildDespatch(c.signer, d)
}

func buildDespatch(signer sign.Signer, d model.Despatch) ([]byte, error) {
	xml, err := ubl.Despatch(d)
	if err != nil {
		return nil, fmt.Errorf("sunat: build despatch: %w", err)
	}
	signed, err := signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign despatch: %w", err)
	}
	return signed, nil
}

// GREClient builds, signs, and sends Guías de Remisión Electrónicas using the
// supplied dependencies.
type GREClient struct {
	signer sign.Signer
	tr     gre.Transport
}

// NewGRE returns a client using signer and tr, without making any I/O calls.
func NewGRE(signer sign.Signer, tr gre.Transport) *GREClient {
	return &GREClient{signer: signer, tr: tr}
}

// DespatchResult holds the signed GRE XML and the ticket SUNAT issued for it.
type DespatchResult struct {
	XML    []byte     // Signed guide; available even when sending fails.
	Ticket gre.Ticket // Zero when sending failed.
}

// DespatchStatusResult is SUNAT's answer for a ticket.
type DespatchStatusResult struct {
	// Status carries the SUNAT code. While Status.InProcess() is true the
	// guide is still being processed and the ticket should be queried again;
	// a rejected guide (Status.Rejected()) carries SUNAT's error in Status.Error.
	Status gre.Status
	// CDR is the parsed constancia, set when SUNAT returned one.
	CDR cdr.Response
}

// BuildDespatch renders and signs d (type 09 or 31) without sending it or
// performing I/O.
func (c *GREClient) BuildDespatch(d model.Despatch) ([]byte, error) {
	return buildDespatch(c.signer, d)
}

// SendDespatch builds and signs d, uploads it with ctx, and returns the ticket.
// SUNAT validates the guide asynchronously: pass the ticket to DespatchStatus
// to obtain the CDR. Once signing succeeds, DespatchResult.XML is returned
// even on error so callers can store it. Transport errors retain their type
// (errors.As finds *gre.Error).
func (c *GREClient) SendDespatch(ctx context.Context, d model.Despatch) (DespatchResult, error) {
	xml, err := c.BuildDespatch(d)
	if err != nil {
		return DespatchResult{}, err
	}
	result := DespatchResult{XML: xml}
	result.Ticket, err = c.tr.Send(ctx, d.FileName(), xml)
	if err != nil {
		return result, fmt.Errorf("sunat: send despatch: %w", err)
	}
	return result, nil
}

// DespatchStatus queries ticket and parses the CDR when SUNAT returns one.
//
// A guide still in process or rejected by SUNAT is a result, not an error:
// check result.Status. When the CDR cannot be parsed the error is returned
// together with the status, whose CDRZip still holds the raw archive.
func (c *GREClient) DespatchStatus(ctx context.Context, ticket string) (DespatchStatusResult, error) {
	st, err := c.tr.Status(ctx, ticket)
	if err != nil {
		return DespatchStatusResult{}, fmt.Errorf("sunat: despatch status: %w", err)
	}
	result := DespatchStatusResult{Status: st}
	if len(st.CDRZip) == 0 {
		return result, nil
	}
	result.CDR, err = cdr.ParseZip(st.CDRZip)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	return result, nil
}
