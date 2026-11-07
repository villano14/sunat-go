package model_test

import (
	"testing"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

func TestDespatchIDAndFileName(t *testing.T) {
	for _, tt := range []struct {
		name     string
		despatch model.Despatch
		id       string
		file     string
	}{
		{
			"remitente",
			model.Despatch{Type: catalog.DespatchSender, Series: "T001", Number: 123, Company: model.Company{RUC: "20000000001"}},
			"T001-123", "20000000001-09-T001-123",
		},
		{
			"transportista",
			model.Despatch{Type: catalog.DespatchCarrier, Series: "V001", Number: 7, Company: model.Company{RUC: "20987654321"}},
			"V001-7", "20987654321-31-V001-7",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.despatch.ID(); got != tt.id {
				t.Errorf("ID() = %q, want %q", got, tt.id)
			}
			if got := tt.despatch.FileName(); got != tt.file {
				t.Errorf("FileName() = %q, want %q", got, tt.file)
			}
		})
	}
}
