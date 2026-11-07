package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
)

// Note is a Nota de crédito (07) or Nota de débito (08). It modifies one
// previously issued Factura or Boleta, identified by AffectedType and
// AffectedNumber. Its lines reuse InvoiceLine.
//
// Reason is a catalog 09 code for a credit note and a catalog 10 code for a
// debit note.
type Note struct {
	Type      catalog.DocumentType // catalog.CreditNote or catalog.DebitNote
	Series    string               // F... for a note over a factura, B... over a boleta
	Number    int64
	IssueDate time.Time
	Currency  string

	// Document being modified.
	AffectedType   catalog.DocumentType // 01 or 03
	AffectedNumber string               // e.g. "F001-45"
	Reason         string               // catalog 09 (credit) or 10 (debit) code
	Description    string               // free text explaining the reason

	Company      Company
	Client       Client
	PaymentTerms *PaymentTerms

	TaxedAmount      decimal.Decimal
	ExoneratedAmount decimal.Decimal
	UnaffectedAmount decimal.Decimal
	ExportAmount     decimal.Decimal
	FreeAmount       decimal.Decimal
	FreeIGV          decimal.Decimal
	IGV              decimal.Decimal
	ISC              decimal.Decimal
	ISCBase          decimal.Decimal
	IVAP             decimal.Decimal
	IVAPBase         decimal.Decimal
	OtherTaxes       decimal.Decimal
	OtherTaxesBase   decimal.Decimal
	ICBPER           decimal.Decimal
	TotalTaxes       decimal.Decimal
	ChargeTotal      decimal.Decimal
	Rounding         decimal.Decimal
	Payable          decimal.Decimal // Importe total de la nota

	OrderReference string
	DespatchRefs   []DocumentRef
	RelatedDocs    []DocumentRef

	Lines   []InvoiceLine
	Legends []Legend
}

// ID returns the document identifier, e.g. "FC01-3".
func (n Note) ID() string {
	return n.Series + "-" + strconv.FormatInt(n.Number, 10)
}

// FileName returns the SUNAT file name without extension, e.g. "20000000001-07-FC01-3".
func (n Note) FileName() string {
	return n.Company.RUC + "-" + string(n.Type) + "-" + n.ID()
}
