package model_test

import (
	"testing"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

func TestInvoiceID(t *testing.T) {
	inv := model.Invoice{Series: "F001", Number: 123}
	if got := inv.ID(); got != "F001-123" {
		t.Errorf("ID() = %q, want %q", got, "F001-123")
	}
}

func TestInvoiceFileName(t *testing.T) {
	inv := model.Invoice{
		Type:    catalog.Invoice,
		Series:  "F001",
		Number:  1,
		Company: model.Company{RUC: "20000000001"},
	}
	if got := inv.FileName(); got != "20000000001-01-F001-1" {
		t.Errorf("FileName() = %q, want %q", got, "20000000001-01-F001-1")
	}
}
