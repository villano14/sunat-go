package sunat

import (
	"context"
	"fmt"

	"github.com/villano14/sunat-go/cdr"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

// BuildNote renders and signs a Nota de crédito (07) or débito (08) without
// sending it or performing I/O.
func (c *Client) BuildNote(n model.Note) ([]byte, error) {
	xml, err := ubl.Note(n)
	if err != nil {
		return nil, fmt.Errorf("sunat: build note: %w", err)
	}
	signed, err := c.signer.Sign(xml)
	if err != nil {
		return nil, fmt.Errorf("sunat: sign note: %w", err)
	}
	return signed, nil
}

// SendNote builds, signs and sends n, then parses the CDR. It follows the same
// contract as SendInvoice: Result.XML is returned once signing succeeds, even
// on error, and a rejected CDR is a response, not an error.
func (c *Client) SendNote(ctx context.Context, n model.Note) (Result, error) {
	xml, err := c.BuildNote(n)
	if err != nil {
		return Result{}, err
	}
	result := Result{XML: xml}
	zipped, err := c.tr.SendBill(ctx, n.FileName(), xml)
	if err != nil {
		return result, fmt.Errorf("sunat: send note: %w", err)
	}
	result.CDR, err = cdr.ParseZip(zipped)
	if err != nil {
		return result, fmt.Errorf("sunat: parse CDR: %w", err)
	}
	return result, nil
}
