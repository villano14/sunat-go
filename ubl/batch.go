package ubl

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

// batchMaxCorrelative is the largest correlative SUNAT accepts in the file name.
const batchMaxCorrelative = 99999

type batchSummaryView struct {
	ID             string
	GenerationDate time.Time
	IssueDate      time.Time
	Company        model.Company
	Currency       string
	SignatureID    string
	Lines          []batchSummaryLineView
}

type batchSummaryLineView struct {
	model.SummaryLine
	Payments []batchPaymentView
	Taxes    []batchTaxView
}

type batchPaymentView struct {
	Amount      decimal.Decimal
	Instruction string
}

type batchTaxView struct {
	Amount      decimal.Decimal
	Scheme      catalog.TaxScheme
	ShowPercent bool
	Percent     decimal.Decimal
}

type batchVoidedView struct {
	ID             string
	GenerationDate time.Time
	IssueDate      time.Time
	Company        model.Company
	SignatureID    string
	Lines          []model.VoidedLine
}

// Summary renders a Resumen diario (RC) as unsigned SummaryDocuments-1 XML.
//
// Per line, the operation amounts (billing payments 01 to 05) and other
// charges are omitted when zero, except that code 01 is always reported when
// no other operation is. The tax totals follow Greenter: IVAP replaces IGV
// when set, and ISC, other taxes and ICBPER are added when non-zero.
func Summary(s model.Summary) ([]byte, error) {
	if err := batchCheck("summary", s.Correlative, s.GenerationDate, s.IssueDate, len(s.Lines)); err != nil {
		return nil, err
	}
	v := batchSummaryView{
		ID:             s.ID(),
		GenerationDate: s.GenerationDate,
		IssueDate:      s.IssueDate,
		Company:        s.Company,
		Currency:       s.Currency,
		SignatureID:    SignatureID,
	}
	if v.Currency == "" {
		v.Currency = "PEN"
	}
	for i, l := range s.Lines {
		if l.Type == "" || l.Series == "" {
			return nil, fmt.Errorf("ubl: summary line %d: document type and series are required", i+1)
		}
		if !l.Status.Valid() {
			return nil, fmt.Errorf("ubl: summary line %d: unsupported status %q", i+1, string(l.Status))
		}
		if l.Perception != nil {
			if _, err := catalog.PerceptionRate(l.Perception.Regime); err != nil {
				return nil, fmt.Errorf("ubl: summary line %d: %w", i+1, err)
			}
		}
		v.Lines = append(v.Lines, batchSummaryLineView{
			SummaryLine: l,
			Payments:    batchPayments(l),
			Taxes:       batchTaxes(l),
		})
	}
	return render("summary.xml.tmpl", v)
}

// Voided renders a Comunicación de baja (RA) as unsigned VoidedDocuments-1 XML.
func Voided(vd model.Voided) ([]byte, error) {
	if err := batchCheck("voided", vd.Correlative, vd.GenerationDate, vd.IssueDate, len(vd.Lines)); err != nil {
		return nil, err
	}
	for i, l := range vd.Lines {
		if l.Type == "" || l.Series == "" || l.Number < 1 {
			return nil, fmt.Errorf("ubl: voided line %d: document type, series and number are required", i+1)
		}
		if l.Reason == "" {
			return nil, fmt.Errorf("ubl: voided line %d: void reason is required", i+1)
		}
	}
	return render("voided.xml.tmpl", batchVoidedView{
		ID:             vd.ID(),
		GenerationDate: vd.GenerationDate,
		IssueDate:      vd.IssueDate,
		Company:        vd.Company,
		SignatureID:    SignatureID,
		Lines:          vd.Lines,
	})
}

func batchCheck(kind string, correlative int, generation, issue time.Time, lines int) error {
	switch {
	case correlative < 1 || correlative > batchMaxCorrelative:
		return fmt.Errorf("ubl: %s: correlative %d out of range 1..%d", kind, correlative, batchMaxCorrelative)
	case generation.IsZero() || issue.IsZero():
		return fmt.Errorf("ubl: %s: generation and issue dates are required", kind)
	case lines == 0:
		return errors.New("ubl: " + kind + ": at least one line is required")
	}
	return nil
}

func batchPayments(l model.SummaryLine) []batchPaymentView {
	others := []batchPaymentView{
		{l.ExoneratedAmount, catalog.PaymentExonerated},
		{l.UnaffectedAmount, catalog.PaymentUnaffected},
		{l.ExportAmount, catalog.PaymentExport},
		{l.FreeAmount, catalog.PaymentFree},
	}
	var out []batchPaymentView
	for _, p := range others {
		if !p.Amount.IsZero() {
			out = append(out, p)
		}
	}
	if !l.TaxedAmount.IsZero() || len(out) == 0 {
		out = append([]batchPaymentView{{l.TaxedAmount, catalog.PaymentTaxed}}, out...)
	}
	return out
}

func batchTaxes(l model.SummaryLine) []batchTaxView {
	var out []batchTaxView
	if !l.IVAP.IsZero() {
		out = append(out, batchTaxView{Amount: l.IVAP, Scheme: catalog.SchemeIVAP})
	} else {
		out = append(out, batchTaxView{
			Amount: l.IGV, Scheme: catalog.SchemeIGV,
			ShowPercent: !l.IGVPercent.IsZero(), Percent: l.IGVPercent,
		})
	}
	for _, t := range []batchTaxView{
		{Amount: l.ISC, Scheme: catalog.SchemeISC},
		{Amount: l.OtherTaxes, Scheme: catalog.SchemeOthers},
		{Amount: l.ICBPER, Scheme: catalog.SchemeICBPER},
	} {
		if !t.Amount.IsZero() {
			out = append(out, t)
		}
	}
	return out
}
