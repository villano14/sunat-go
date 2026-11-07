package model_test

import (
	"testing"
	"time"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

func TestSummaryIDAndFileName(t *testing.T) {
	// The ID carries the summary issue date, not the date of the documents.
	s := model.Summary{
		Company:        model.Company{RUC: "20000000001"},
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC),
		Correlative:    12,
	}
	if got := s.ID(); got != "RC-20261028-12" {
		t.Errorf("ID() = %q, want RC-20261028-12", got)
	}
	if got := s.FileName(); got != "20000000001-RC-20261028-12" {
		t.Errorf("FileName() = %q", got)
	}
}

func TestVoidedIDAndFileName(t *testing.T) {
	v := model.Voided{
		Company:        model.Company{RUC: "20000000001"},
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC),
		Correlative:    1,
	}
	if got := v.ID(); got != "RA-20261028-1" {
		t.Errorf("ID() = %q, want RA-20261028-1", got)
	}
	if got := v.FileName(); got != "20000000001-RA-20261028-1" {
		t.Errorf("FileName() = %q", got)
	}
}

func TestBatchLineIDs(t *testing.T) {
	if got := (model.SummaryLine{Type: catalog.Receipt, Series: "B001", Number: 7}).ID(); got != "B001-7" {
		t.Errorf("SummaryLine.ID() = %q", got)
	}
	if got := (model.SummaryReference{Type: catalog.Receipt, Series: "B001", Number: 1}).ID(); got != "B001-1" {
		t.Errorf("SummaryReference.ID() = %q", got)
	}
	if got := (model.VoidedLine{Type: catalog.Invoice, Series: "F001", Number: 12}).ID(); got != "F001-12" {
		t.Errorf("VoidedLine.ID() = %q", got)
	}
}

func TestSummaryStatusValid(t *testing.T) {
	for _, s := range []catalog.SummaryStatus{catalog.SummaryAdd, catalog.SummaryModify, catalog.SummaryVoid} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	for _, s := range []catalog.SummaryStatus{"", "0", "4"} {
		if s.Valid() {
			t.Errorf("%q must be invalid", s)
		}
	}
}
