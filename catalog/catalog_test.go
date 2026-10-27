package catalog_test

import (
	"testing"

	"github.com/villano14/sunat-go/catalog"
)

func TestSchemeFor(t *testing.T) {
	tests := []struct {
		affectation catalog.IGVAffectation
		want        catalog.TaxScheme
	}{
		{catalog.TaxedOnerous, catalog.SchemeIGV},
		{catalog.TaxedFree, catalog.SchemeFree},
		{catalog.ExoneratedOnerous, catalog.SchemeExonerated},
		{catalog.UnaffectedOnerous, catalog.SchemeUnaffected},
		{catalog.UnaffectedFree, catalog.SchemeFree},
		{catalog.Export, catalog.SchemeExport},
	}
	for _, tt := range tests {
		got, err := catalog.SchemeFor(tt.affectation)
		if err != nil {
			t.Fatalf("SchemeFor(%q) error: %v", tt.affectation, err)
		}
		if got != tt.want {
			t.Errorf("SchemeFor(%q) = %+v, want %+v", tt.affectation, got, tt.want)
		}
	}
}

func TestSchemeForUnknown(t *testing.T) {
	if _, err := catalog.SchemeFor("99"); err == nil {
		t.Fatal("SchemeFor(\"99\") expected error, got nil")
	}
}

func TestSchemeValues(t *testing.T) {
	if catalog.SchemeIGV != (catalog.TaxScheme{ID: "1000", Name: "IGV", Code: "VAT"}) {
		t.Errorf("SchemeIGV = %+v", catalog.SchemeIGV)
	}
	if catalog.SchemeFree != (catalog.TaxScheme{ID: "9996", Name: "GRA", Code: "FRE"}) {
		t.Errorf("SchemeFree = %+v", catalog.SchemeFree)
	}
}
