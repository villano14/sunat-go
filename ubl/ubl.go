// Package ubl renders SUNAT documents as UBL 2.1 XML.
//
// The output is unsigned: it carries an empty ext:ExtensionContent where the
// sign package places the XMLDSig signature.
package ubl

import (
	"bytes"
	"embed"
	"encoding/xml"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/shopspring/decimal"
)

// SignatureID is the ds:Signature Id referenced from cac:Signature.
const SignatureID = "SUNATGO-SIGN"

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"x":     escape,
	"amt":   amount,
	"price": price,
	"qty":   quantity,
	"nz":    nonZero,
	"date":  formatDate,
	"time":  func(t time.Time) string { return t.Format("15:04:05") },
	"def":   orDefault,
	"seq":   func(i int) string { return fmt.Sprintf("%03d", i+1) },
	"add1":  func(i int) int { return i + 1 },
}).ParseFS(templateFS, "templates/*.tmpl"))

func render(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, fmt.Errorf("ubl: render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// amount formats a monetary total with exactly two decimals.
func amount(d decimal.Decimal) string { return d.StringFixed(2) }

// price formats a unit price or value: at least 2 and at most 10 decimals.
func price(d decimal.Decimal) string {
	s := d.Round(10).String()
	if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i-1 > 2 {
		return s
	}
	return d.StringFixed(2)
}

// quantity formats a quantity with at most 10 decimals.
func quantity(d decimal.Decimal) string { return d.Round(10).String() }

// formatDate accepts time.Time or *time.Time so optional dates render directly.
func formatDate(v any) (string, error) {
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02"), nil
	case *time.Time:
		return t.Format("2006-01-02"), nil
	}
	return "", fmt.Errorf("date: unsupported type %T", v)
}

func nonZero(d decimal.Decimal) bool { return !d.IsZero() }

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
