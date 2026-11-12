package ubl

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// withholdingView feeds both retention.xml.tmpl and perception.xml.tmpl: the
// two documents share a shape and differ only in element names.
type withholdingView struct {
	ID          string
	SignatureID string
	IssueDate   time.Time
	Company     model.Company
	Party       model.Client // Supplier (retention) or customer (perception)
	Regime      string
	Rate        decimal.Decimal
	Note        string
	Total       decimal.Decimal // Retained or perceived
	Settled     decimal.Decimal // Paid or collected
	Details     []withholdingDetailView
}

type withholdingDetailView struct {
	DocumentType catalog.DocumentType
	Number       string
	IssueDate    time.Time
	Total        decimal.Decimal
	Currency     string
	Payments     []model.Payment

	HasInfo    bool // Retention/perception block is rendered
	InfoAmount decimal.Decimal
	InfoDate   time.Time
	InfoNet    decimal.Decimal
	Exchange   *withholdingExchangeView
}

type withholdingExchangeView struct {
	model.ExchangeRate
	Factor string // Rate with six decimals
}

func newWithholdingExchange(e *model.ExchangeRate) *withholdingExchangeView {
	if e == nil {
		return nil
	}
	return &withholdingExchangeView{ExchangeRate: *e, Factor: e.Rate.StringFixed(6)}
}

// errManyPayments: the SUNAT schema (SUNATRetentionDocumentReferenceType and
// SUNATPerceptionDocumentReferenceType) allows at most one cac:Payment per
// related document. Greenter renders every payment, producing XML the schema
// rejects; each payment must be reported as its own detail instead.
var errManyPayments = errors.New("at most one payment per related document; report each payment as its own detail")

// withholdingInfo reports whether the retention/perception block is rendered,
// which Greenter only does when amount, net and date are all present.
func withholdingInfo(amount, net decimal.Decimal, date time.Time) bool {
	return !amount.IsZero() && !net.IsZero() && !date.IsZero()
}

// Retention renders a Comprobante de Retención (20) as unsigned UBL XML.
//
// Regime must be a catalog 23 code; a zero Rate defaults to the regime's rate.
// The retention block of a detail is omitted when its date or amounts are zero.
func Retention(r model.Retention) ([]byte, error) {
	rate, err := catalog.RetentionRate(r.Regime)
	if err != nil {
		return nil, fmt.Errorf("ubl: retention: %w", err)
	}
	if !r.Rate.IsZero() {
		rate = r.Rate
	}
	v := withholdingView{
		ID: r.ID(), SignatureID: SignatureID, IssueDate: r.IssueDate,
		Company: r.Company, Party: r.Supplier, Regime: string(r.Regime), Rate: rate,
		Note: r.Note, Total: r.TotalRetained, Settled: r.TotalPaid,
	}
	for i, d := range r.Details {
		if len(d.Payments) > 1 {
			return nil, fmt.Errorf("ubl: retention detail %d: %w", i+1, errManyPayments)
		}
		v.Details = append(v.Details, withholdingDetailView{
			DocumentType: d.DocumentType, Number: d.Number, IssueDate: d.IssueDate,
			Total: d.Total, Currency: d.Currency, Payments: d.Payments,
			HasInfo:    withholdingInfo(d.Retained, d.NetPayable, d.RetentionDate),
			InfoAmount: d.Retained, InfoDate: d.RetentionDate, InfoNet: d.NetPayable,
			Exchange: newWithholdingExchange(d.Exchange),
		})
	}
	return render("retention.xml.tmpl", v)
}

// Perception renders a Comprobante de Percepción (40) as unsigned UBL XML.
//
// Regime must be a catalog 22 code; a zero Rate defaults to the regime's rate.
// The perception block of a detail is omitted when its date or amounts are zero.
func Perception(p model.Perception) ([]byte, error) {
	rate, err := catalog.PerceptionRate(p.Regime)
	if err != nil {
		return nil, fmt.Errorf("ubl: perception: %w", err)
	}
	if !p.Rate.IsZero() {
		rate = p.Rate
	}
	v := withholdingView{
		ID: p.ID(), SignatureID: SignatureID, IssueDate: p.IssueDate,
		Company: p.Company, Party: p.Customer, Regime: string(p.Regime), Rate: rate,
		Note: p.Note, Total: p.TotalPerceived, Settled: p.TotalCollected,
	}
	for i, d := range p.Details {
		if len(d.Collections) > 1 {
			return nil, fmt.Errorf("ubl: perception detail %d: %w", i+1, errManyPayments)
		}
		v.Details = append(v.Details, withholdingDetailView{
			DocumentType: d.DocumentType, Number: d.Number, IssueDate: d.IssueDate,
			Total: d.Total, Currency: d.Currency, Payments: d.Collections,
			HasInfo:    withholdingInfo(d.Perceived, d.NetCollectable, d.PerceptionDate),
			InfoAmount: d.Perceived, InfoDate: d.PerceptionDate, InfoNet: d.NetCollectable,
			Exchange: newWithholdingExchange(d.Exchange),
		})
	}
	return render("perception.xml.tmpl", v)
}
