package ubl_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func sampleReversion() model.Reversion {
	return model.Reversion{
		Company:        batchCompany(),
		GenerationDate: time.Date(2026, 10, 27, 0, 0, 0, 0, batchPET),
		IssueDate:      time.Date(2026, 10, 28, 9, 0, 0, 0, batchPET),
		Correlative:    3,
		Lines: []model.VoidedLine{
			{Type: catalog.Retention, Series: "R001", Number: 12, Reason: "ERROR EN <TASA> & MONTO"},
			{Type: catalog.Perception, Series: "P001", Number: 4, Reason: "REVERSION"},
		},
	}
}

func TestReversionGolden(t *testing.T) {
	out, err := ubl.Reversion(sampleReversion())
	if err != nil {
		t.Fatalf("Reversion() error: %v", err)
	}
	batchGolden(t, "reversion-basic.xml", out)
}

func TestReversionValues(t *testing.T) {
	r := sampleReversion()
	if r.ID() != "RR-20261028-3" || r.FileName() != "20000000001-RR-20261028-3" {
		t.Fatalf("ID/FileName = %q / %q", r.ID(), r.FileName())
	}
	out, err := ubl.Reversion(r)
	if err != nil {
		t.Fatalf("Reversion() error: %v", err)
	}
	root := parse(t, out)
	if root.XMLName.Local != "VoidedDocuments" || root.XMLName.Space != "urn:sunat:names:specification:ubl:peru:schema:xsd:VoidedDocuments-1" {
		t.Fatalf("root = %v", root.XMLName)
	}
	for _, p := range []struct {
		want string
		path []string
	}{
		{"2.0", []string{"UBLVersionID"}},
		{"1.0", []string{"CustomizationID"}},
		{"RR-20261028-3", []string{"ID"}},
		{"2026-10-27", []string{"ReferenceDate"}},
		{"2026-10-28", []string{"IssueDate"}},
		{"#SUNATGO-SIGN", []string{"Signature", "DigitalSignatureAttachment", "ExternalReference", "URI"}},
	} {
		if got := batchTexts(root, p.path...); len(got) != 1 || got[0] != p.want {
			t.Errorf("%s = %q, want %q", strings.Join(p.path, "/"), got, p.want)
		}
	}
	lines := batchChildren(root, "VoidedDocumentsLine")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for i, want := range [][]string{
		{"1", "20", "R001", "12", "ERROR EN <TASA> & MONTO"},
		{"2", "40", "P001", "4", "REVERSION"},
	} {
		l := lines[i]
		got := []string{
			batchTexts(l, "LineID")[0], batchTexts(l, "DocumentTypeCode")[0], batchTexts(l, "DocumentSerialID")[0],
			batchTexts(l, "DocumentNumberID")[0], batchTexts(l, "VoidReasonDescription")[0],
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("line %d = %q, want %q", i+1, got, want)
		}
	}
}

func TestReversionValidation(t *testing.T) {
	for name, mutate := range map[string]func(*model.Reversion){
		"no lines":         func(r *model.Reversion) { r.Lines = nil },
		"zero correlative": func(r *model.Reversion) { r.Correlative = 0 },
		"no issue date":    func(r *model.Reversion) { r.IssueDate = time.Time{} },
		"no reason":        func(r *model.Reversion) { r.Lines[0].Reason = "" },
		"no number":        func(r *model.Reversion) { r.Lines[0].Number = 0 },
		"no series":        func(r *model.Reversion) { r.Lines[0].Series = "" },
		"invoice line":     func(r *model.Reversion) { r.Lines[0].Type = catalog.Invoice },
		"empty type":       func(r *model.Reversion) { r.Lines[1].Type = "" },
	} {
		t.Run(name, func(t *testing.T) {
			r := sampleReversion()
			mutate(&r)
			if _, err := ubl.Reversion(r); err == nil || !strings.HasPrefix(err.Error(), "ubl: reversion") {
				t.Fatalf("error = %v, want ubl reversion error", err)
			}
		})
	}
}

func TestReversionAcceptsPurchaseSettlement(t *testing.T) {
	r := sampleReversion()
	r.Lines[1].Type = catalog.PurchaseSettlement
	if _, err := ubl.Reversion(r); err != nil {
		t.Fatalf("Reversion() of a 04 line: %v", err)
	}
}

func TestVoidedRejectsWithholdings(t *testing.T) {
	for _, typ := range []catalog.DocumentType{catalog.Retention, catalog.Perception, catalog.PurchaseSettlement} {
		v := batchSampleVoided()
		v.Lines[1].Type = typ
		_, err := ubl.Voided(v)
		if err == nil || !strings.Contains(err.Error(), "Reversion") {
			t.Errorf("type %s: error = %v, want a pointer to Reversion", typ, err)
		}
	}
}
