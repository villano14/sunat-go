package sunat

import (
	"context"
	"fmt"

	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

// Retentions (20) and perceptions (40) are sent to SUNAT's "otros CPE"
// billService, not the one used by invoices. A Client holds a single
// transport, so callers need a second Client whose transport points at
// transport.OtrosCPEBeta or transport.OtrosCPEProduction:
//
//	invoices := sunat.New(signer, transport.New(transport.BillBeta, creds))
//	others := sunat.New(signer, transport.New(transport.OtrosCPEBeta, creds))
//	result, err := others.SendRetention(ctx, ret)
//
// The signer can be shared between both clients.

// BuildRetention renders and signs r without sending it or performing I/O.
func (c *Client) BuildRetention(r model.Retention) ([]byte, error) {
	xml, err := ubl.Retention(r)
	if err != nil {
		return nil, fmt.Errorf("sunat: build retention: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign retention: %w", err)
	}
	return signed, nil
}

// BuildPerception renders and signs p without sending it or performing I/O.
func (c *Client) BuildPerception(p model.Perception) ([]byte, error) {
	xml, err := ubl.Perception(p)
	if err != nil {
		return nil, fmt.Errorf("sunat: build perception: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign perception: %w", err)
	}
	return signed, nil
}

// SendRetention builds and signs r, sends it with ctx, and parses the CDR.
// The Client's transport must target the otros CPE endpoint
// (transport.OtrosCPEBeta or transport.OtrosCPEProduction).
//
// Error and Result semantics match SendInvoice: once signing succeeds,
// Result.XML is returned even on error, a rejected CDR is a response rather
// than an error, and transport errors retain their type.
func (c *Client) SendRetention(ctx context.Context, r model.Retention) (Result, error) {
	xml, err := c.BuildRetention(r)
	if err != nil {
		return Result{}, err
	}
	return c.sendWithholding(ctx, "retention", r.FileName(), xml)
}

// SendPerception builds and signs p, sends it with ctx, and parses the CDR.
// The Client's transport must target the otros CPE endpoint
// (transport.OtrosCPEBeta or transport.OtrosCPEProduction).
//
// Error and Result semantics match SendRetention.
func (c *Client) SendPerception(ctx context.Context, p model.Perception) (Result, error) {
	xml, err := c.BuildPerception(p)
	if err != nil {
		return Result{}, err
	}
	return c.sendWithholding(ctx, "perception", p.FileName(), xml)
}

func (c *Client) sendWithholding(ctx context.Context, kind, fileName string, signed []byte) (Result, error) {
	result := Result{XML: signed}
	zipped, err := c.tr.SendBill(ctx, fileName, signed)
	if err != nil {
		return result, fmt.Errorf("sunat: send %s: %w", kind, err)
	}
	result.CDR, err = cdr.ParseZip(zipped)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	return result, nil
}
