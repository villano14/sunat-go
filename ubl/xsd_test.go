package ubl_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/villano14/sunat-go/catalog"
	"github.com/villano14/sunat-go/internal/testcert"
	"github.com/villano14/sunat-go/model"
	"github.com/villano14/sunat-go/sign"
	"github.com/villano14/sunat-go/ubl"
)

// signForXSD signs out with a throwaway certificate: UBL 2.1 requires
// ext:ExtensionContent to hold an element, so unsigned output is not valid.
func signForXSD(t *testing.T, out []byte) []byte {
	t.Helper()
	key, cert := testcert.New(t)
	signed, err := sign.New(sign.Certificate{Key: key, Cert: cert}).Sign(out)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// validateXSD validates out with xmllint against a schema of the local,
// git-ignored cache (scripts/fetch-xsd.sh). Missing xmllint or cached schemas
// skip the test; there is no network fallback.
func validateXSD(t *testing.T, schema string, out []byte) {
	t.Helper()
	bin, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed")
	}
	schema = filepath.Join("..", "testdata-xsd", schema)
	requireSummarySchema(t, schema, make(map[string]bool))
	doc := filepath.Join(t.TempDir(), "doc.xml")
	if err := os.WriteFile(doc, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(bin, "--nonet", "--noout", "--schema", schema, doc).CombinedOutput(); err != nil {
		t.Fatalf("xmllint rejected %s: %v\n%s", filepath.Base(schema), err, output)
	}
}

func TestDocumentsXSD(t *testing.T) {
	for _, c := range []struct {
		name, schema, golden string
	}{
		{"invoice", "2.1/maindoc/UBL-Invoice-2.1.xsd", "invoice-basic.xml"},
		{"despatch", "2.1/maindoc/UBL-DespatchAdvice-2.1.xsd", "despatch-basic.xml"},
		{"despatch transportista", "2.1/maindoc/UBL-DespatchAdvice-2.1.xsd", "despatch-transportista.xml"},
		{"retention", "2.0/maindoc/UBL-Retention-2.0.xsd", "retention-basic.xml"},
		{"perception", "2.0/maindoc/UBL-Perception-2.0.xsd", "perception-basic.xml"},
		{"voided", "2.0/maindoc/UBL-VoidedDocuments-2.0.xsd", "voided-basic.xml"},
		{"reversion", "2.0/maindoc/UBL-VoidedDocuments-2.0.xsd", "reversion-basic.xml"},
		{"summary", "2.0/maindoc/UBL-SummaryDocuments-2.0.xsd", "summary-basic.xml"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := os.ReadFile(filepath.Join("testdata", c.golden))
			if err != nil {
				t.Fatal(err)
			}
			validateXSD(t, c.schema, signForXSD(t, out))
		})
	}
}

// TestRenderedXSD validates documents rendered with optional parts that the
// goldens do not cover.
func TestRenderedXSD(t *testing.T) {
	credit := sampleInvoice()
	credit.PaymentTerms = &model.PaymentTerms{
		Credit: true, Amount: d("118"),
		Installments: []model.Installment{{Amount: d("118"), DueDate: credit.IssueDate.AddDate(0, 1, 0)}},
	}
	render := func(f func() ([]byte, error)) []byte {
		t.Helper()
		out, err := f()
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, c := range []struct {
		name, schema string
		out          func() ([]byte, error)
	}{
		{"credit note", "2.1/maindoc/UBL-CreditNote-2.1.xsd", func() ([]byte, error) { return ubl.Note(sampleNote(catalog.CreditNote)) }},
		{"debit note", "2.1/maindoc/UBL-DebitNote-2.1.xsd", func() ([]byte, error) { return ubl.Note(sampleNote(catalog.DebitNote)) }},
		{"invoice on credit", "2.1/maindoc/UBL-Invoice-2.1.xsd", func() ([]byte, error) { return ubl.Invoice(credit) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			validateXSD(t, c.schema, signForXSD(t, render(c.out)))
		})
	}
}
