package sunat

import (
	"context"
	"fmt"

	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

// BuildPurchaseSettlement renders and signs a Liquidación de compra (04)
// without sending it or performing I/O.
func (c *Client) BuildPurchaseSettlement(p model.PurchaseSettlement) ([]byte, error) {
	xml, err := ubl.PurchaseSettlement(p)
	if err != nil {
		return nil, fmt.Errorf("sunat: build purchase settlement: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign purchase settlement: %w", err)
	}
	return signed, nil
}

// SendPurchaseSettlement builds and signs p, sends it with sendBill to the
// same billService as invoices (transport.BillBeta or BillProduction), and
// parses the CDR. An accepted settlement is reverted with a Reversion (RR),
// not a Voided (RA).
//
// Error and Result semantics match SendInvoice: once signing succeeds,
// Result.XML is returned even on error, a rejected CDR is a response rather
// than an error, and transport errors retain their type.
func (c *Client) SendPurchaseSettlement(ctx context.Context, p model.PurchaseSettlement) (Result, error) {
	xml, err := c.BuildPurchaseSettlement(p)
	if err != nil {
		return Result{}, err
	}
	result := Result{XML: xml}
	zipped, err := c.tr.SendBill(ctx, p.FileName(), xml)
	if err != nil {
		return result, fmt.Errorf("sunat: send purchase settlement: %w", err)
	}
	result.CDR, err = cdr.ParseZip(zipped)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	return result, nil
}
