package ubl_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/ubl"
)

func TestSummaryPerception(t *testing.T) {
	for _, currency := range []string{"", "PEN", "USD"} {
		t.Run(currency, func(t *testing.T) {
			s := batchSampleSummary()
			s.Currency = currency
			s.Lines[0].Perception = &model.SummaryPerception{
				Regime: catalog.PerceptionInternalSale, Rate: d("2"),
				Amount: d("4"), TotalCollected: d("204"), TaxableAmount: d("200"),
			}
			before := *s.Lines[0].Perception
			out, err := ubl.Summary(s)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*s.Lines[0].Perception, before) || s.Currency != currency {
				t.Fatal("render mutated caller input")
			}
			lines := batchChildren(parse(t, out), "SummaryDocumentsLine")
			refs := batchChildren(lines[0], "SUNATPerceptionSummaryDocumentReference")
			if len(refs) != 1 {
				t.Fatalf("perception references = %d; want 1\n%s", len(refs), out)
			}
			if len(batchChildren(lines[1], "SUNATPerceptionSummaryDocumentReference")) != 0 {
				t.Fatal("perception leaked into another line")
			}
			ref := refs[0]
			sac := "urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1"
			cbc := "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
			if ref.XMLName.Space != sac {
				t.Fatalf("reference namespace=%s", ref.XMLName.Space)
			}
			wantNames := []xml.Name{
				{Space: sac, Local: "SUNATPerceptionSystemCode"},
				{Space: sac, Local: "SUNATPerceptionPercent"},
				{Space: cbc, Local: "TotalInvoiceAmount"},
				{Space: sac, Local: "SUNATTotalCashed"},
				{Space: cbc, Local: "TaxableAmount"},
			}
			wantValues := []string{"01", "2.00", "4.00", "204.00", "200.00"}
			if len(ref.Nodes) != len(wantNames) {
				t.Fatalf("perception children=%v", ref.Nodes)
			}
			cur := currency
			if cur == "" {
				cur = "PEN"
			}
			for i, c := range ref.Nodes {
				if c.XMLName != wantNames[i] || c.Text != wantValues[i] {
					t.Errorf("child %d=%v %q", i, c.XMLName, c.Text)
				}
				if i >= 2 && (len(c.Attrs) != 1 || c.Attrs[0].Name.Local != "currencyID" || c.Attrs[0].Value != cur) {
					t.Errorf("child %d attributes=%v", i, c.Attrs)
				}
			}
			// The cached SUNAT schema requires Perception before Status/TotalAmount.
			var order []string
			for _, c := range lines[0].Nodes {
				order = append(order, c.XMLName.Local)
			}
			wantOrder := []string{"LineID", "DocumentTypeCode", "ID", "AccountingCustomerParty", "SUNATPerceptionSummaryDocumentReference", "Status", "TotalAmount", "BillingPayment", "AllowanceCharge", "TaxTotal", "TaxTotal"}
			if !reflect.DeepEqual(order, wantOrder) {
				t.Errorf("line order=%v", order)
			}
		})
	}
}

// requireSummarySchema checks the complete local import/include graph before
// invoking xmllint. Missing prerequisites skip; malformed cached schemas fail.
// No download or network fallback is permitted.
func requireSummarySchema(t *testing.T, path string, seen map[string]bool) {
	t.Helper()
	path = filepath.Clean(path)
	if seen[path] {
		return
	}
	seen[path] = true
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("local XSD cache prerequisite absent: %s", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse cached schema %s: %v", path, err)
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Space != "http://www.w3.org/2001/XMLSchema" {
			continue
		}
		switch element.Name.Local {
		case "import", "include", "redefine":
			for _, attr := range element.Attr {
				if attr.Name.Local == "schemaLocation" && attr.Value != "" {
					requireSummarySchema(t, filepath.Join(filepath.Dir(path), attr.Value), seen)
				}
			}
		}
	}
}

func TestSummaryPerceptionXSD(t *testing.T) {
	bin, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed")
	}
	schema := filepath.Join("..", "testdata-xsd", "2.0", "maindoc", "UBL-SummaryDocuments-2.0.xsd")
	requireSummarySchema(t, schema, make(map[string]bool))
	for _, perception := range []bool{false, true} {
		name := "nil-golden"
		if perception {
			name = "with-perception"
		}
		t.Run(name, func(t *testing.T) {
			s := batchSampleSummary()
			if perception {
				// Exercise placement both with and without BillingReference.
				for i := range s.Lines {
					s.Lines[i].Perception = &model.SummaryPerception{
						Regime: catalog.PerceptionInternalSale, Rate: d("2"),
						Amount: d("4"), TotalCollected: d("204"), TaxableAmount: d("200"),
					}
				}
			}
			out, err := ubl.Summary(s)
			if err != nil {
				t.Fatal(err)
			}
			if !perception {
				batchGolden(t, "summary-basic.xml", out)
			}
			doc := filepath.Join(t.TempDir(), "summary.xml")
			if err := os.WriteFile(doc, out, 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "--nonet", "--noout", "--schema", schema, doc)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("xmllint rejected generated summary: %v\n%s", err, output)
			}
		})
	}
}

func TestSummaryPerceptionAbsentPreservesXML(t *testing.T) {
	out, err := ubl.Summary(batchSampleSummary())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("SUNATPerception")) {
		t.Fatal("nil perception must emit no XML")
	}
	batchGolden(t, "summary-basic.xml", out)
}

func TestSummaryPerceptionRegimeValidation(t *testing.T) {
	s := batchSampleSummary()
	s.Lines[0].Perception = &model.SummaryPerception{Regime: "99", Rate: d("2"), Amount: d("4"), TotalCollected: d("204"), TaxableAmount: d("200")}
	before := *s.Lines[0].Perception
	out, err := ubl.Summary(s)
	if err == nil || err.Error() != `ubl: summary line 1: catalog: unsupported perception regime "99"` || len(out) != 0 {
		t.Fatalf("XML=%q error=%v", out, err)
	}
	if !reflect.DeepEqual(*s.Lines[0].Perception, before) {
		t.Fatal("invalid input mutated")
	}
	// Reuse catalog validation, without inventing amount reconciliation or a
	// summary-specific restriction on already supported catalog regimes.
	for _, regime := range []catalog.PerceptionRegime{catalog.PerceptionInternalSale, catalog.PerceptionFuel, catalog.PerceptionSpecialRate} {
		s.Lines[0].Perception.Regime = regime
		s.Lines[0].Perception.Rate = d("0")
		out, err = ubl.Summary(s)
		if err != nil {
			t.Fatal(err)
		}
		if got := batchTexts(parse(t, out), "SummaryDocumentsLine", "SUNATPerceptionSummaryDocumentReference", "SUNATPerceptionPercent"); !reflect.DeepEqual(got, []string{"0.00"}) {
			t.Fatalf("explicit rate must not be recalculated: %v", got)
		}
	}
}
