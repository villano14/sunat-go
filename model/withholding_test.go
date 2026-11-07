package model_test

import (
	"testing"

	"github.com/villano14/sunat-go/model"
)

func TestRetentionIDAndFileName(t *testing.T) {
	r := model.Retention{Series: "R001", Number: 7, Company: model.Company{RUC: "20000000001"}}
	if got := r.ID(); got != "R001-7" {
		t.Errorf("ID() = %q, want %q", got, "R001-7")
	}
	if got := r.FileName(); got != "20000000001-20-R001-7" {
		t.Errorf("FileName() = %q, want %q", got, "20000000001-20-R001-7")
	}
}

func TestPerceptionIDAndFileName(t *testing.T) {
	p := model.Perception{Series: "P001", Number: 12, Company: model.Company{RUC: "20000000001"}}
	if got := p.ID(); got != "P001-12" {
		t.Errorf("ID() = %q, want %q", got, "P001-12")
	}
	if got := p.FileName(); got != "20000000001-40-P001-12" {
		t.Errorf("FileName() = %q, want %q", got, "20000000001-40-P001-12")
	}
}
