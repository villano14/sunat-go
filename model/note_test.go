package model_test

import (
	"testing"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
)

func TestNoteIdentifiers(t *testing.T) {
	for _, kind := range []catalog.DocumentType{catalog.CreditNote, catalog.DebitNote} {
		for _, series := range []string{"FC01", "BC01"} {
			t.Run(string(kind)+"/"+series, func(t *testing.T) {
				n := model.Note{Type: kind, Series: series, Number: 123, Company: model.Company{RUC: "20000000001"}}
				if got, want := n.ID(), series+"-123"; got != want {
					t.Errorf("ID() = %q, want %q", got, want)
				}
				if got, want := n.FileName(), "20000000001-"+string(kind)+"-"+series+"-123"; got != want {
					t.Errorf("FileName() = %q, want %q", got, want)
				}
			})
		}
	}
}
