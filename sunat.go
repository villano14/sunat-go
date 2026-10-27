// Package sunat provides a client facade for SUNAT electronic invoicing.
//
// The invoice pipeline renders UBL 2.1 XML, signs it, sends it through the
// configured transport, and parses the zipped CDR returned by SUNAT.
// Signers and transports are injected; each package can also be used on its own.
package sunat

import (
	"context"
	"fmt"

	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/sign"
	"github.com/villano14/sunat-go/transport"
	"github.com/villano14/sunat-go/ubl"
)

// Client builds, signs, and sends invoices using the supplied dependencies.
type Client struct {
	signer sign.Signer
	tr     transport.Transport
}

// New returns a client using signer and tr, without making any I/O calls.
func New(signer sign.Signer, tr transport.Transport) *Client {
	return &Client{signer: signer, tr: tr}
}

// Result holds the signed invoice XML and SUNAT's parsed CDR.
type Result struct {
	XML []byte // Signed invoice; available even on send or CDR parse errors.
	CDR cdr.Response
}

// BuildInvoice renders and signs inv without sending it or performing I/O.
func (c *Client) BuildInvoice(inv model.Invoice) ([]byte, error) {
	xml, err := ubl.Invoice(inv)
	if err != nil {
		return nil, fmt.Errorf("sunat: build invoice: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign invoice: %w", err)
	}
	return signed, nil
}

// SendInvoice builds and signs inv, sends it with ctx, and parses the CDR.
// Once signing succeeds, Result.XML is returned even on error so callers can
// store it. A rejected CDR is returned as a response, not an error; callers
// should check Result.CDR.Accepted(). Transport errors retain their type.
func (c *Client) SendInvoice(ctx context.Context, inv model.Invoice) (Result, error) {
	xml, err := c.BuildInvoice(inv)
	if err != nil {
		return Result{}, err
	}
	result := Result{XML: xml}
	zipped, err := c.tr.SendBill(ctx, inv.FileName(), xml)
	if err != nil {
		return result, fmt.Errorf("sunat: send invoice: %w", err)
	}
	result.CDR, err = cdr.ParseZip(zipped)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	return result, nil
}
