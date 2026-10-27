package cdr_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/villano14/sunat-go/cdr"
)

func cdrXML(code, description string, notes ...string) []byte {
	var n string
	for _, note := range notes {
		n += "<cbc:Note>" + note + "</cbc:Note>"
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ar:ApplicationResponse xmlns:ar="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2">
  <ext:UBLExtensions><ext:UBLExtension><ext:ExtensionContent/></ext:UBLExtension></ext:UBLExtensions>
  <cbc:UBLVersionID>2.0</cbc:UBLVersionID>
  <cbc:ID>1761591234567</cbc:ID>
  ` + n + `
  <cac:DocumentResponse>
    <cac:Response>
      <cbc:ReferenceID>F001-1</cbc:ReferenceID>
      <cbc:ResponseCode>` + code + `</cbc:ResponseCode>
      <cbc:Description>` + description + `</cbc:Description>
    </cac:Response>
    <cac:DocumentReference><cbc:ID>F001-1</cbc:ID></cac:DocumentReference>
  </cac:DocumentResponse>
</ar:ApplicationResponse>`)
}

func zipFiles(t *testing.T, files map[string][]byte, order ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(files[name])
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseAccepted(t *testing.T) {
	r, err := cdr.Parse(cdrXML("0", "La Factura numero F001-1, ha sido aceptada"))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if r.Code != "0" || r.ReferenceID != "F001-1" || r.ID != "1761591234567" ||
		r.Description != "La Factura numero F001-1, ha sido aceptada" || len(r.Notes) != 0 {
		t.Errorf("Parse() = %+v", r)
	}
	if !r.Accepted() {
		t.Error("code 0 must be accepted")
	}
}

func TestParseObservations(t *testing.T) {
	r, err := cdr.Parse(cdrXML("0", "aceptada", "4252 - El dato ingresado como atributo @listName es incorrecto.", "4287 - Otro"))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	want := []string{"4252 - El dato ingresado como atributo @listName es incorrecto.", "4287 - Otro"}
	if len(r.Notes) != 2 || r.Notes[0] != want[0] || r.Notes[1] != want[1] {
		t.Errorf("Notes = %q, want %q", r.Notes, want)
	}
	if !r.Accepted() {
		t.Error("observations do not reject the document")
	}
}

func TestAccepted(t *testing.T) {
	tests := map[string]bool{"0": true, "4000": true, "4252": true, "0100": false, "1033": false, "2335": false, "3999": false, "": false, "x": false}
	for code, want := range tests {
		if got := (cdr.Response{Code: code}).Accepted(); got != want {
			t.Errorf("Response{Code: %q}.Accepted() = %v, want %v", code, got, want)
		}
	}
}

func TestParseMissingResponseCode(t *testing.T) {
	if _, err := cdr.Parse([]byte(`<ar:ApplicationResponse xmlns:ar="urn:x"/>`)); err == nil {
		t.Fatal("expected error when ResponseCode is missing")
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := cdr.Parse([]byte("<ar:ApplicationResponse")); err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestParseZipSkipsDummyEntries(t *testing.T) {
	xml := cdrXML("2335", "El documento electrónico ingresado ha sido alterado")
	z := zipFiles(t, map[string][]byte{"dummy/": nil, "R-20000000001-01-F001-1.xml": xml}, "dummy/", "R-20000000001-01-F001-1.xml")
	r, err := cdr.ParseZip(z)
	if err != nil {
		t.Fatalf("ParseZip() error: %v", err)
	}
	if r.Code != "2335" || r.Accepted() {
		t.Errorf("ParseZip() = %+v", r)
	}
	if !bytes.Equal(r.XML, xml) {
		t.Error("Response.XML must hold the raw CDR XML")
	}
}

func TestParseZipWithoutXML(t *testing.T) {
	z := zipFiles(t, map[string][]byte{"readme.txt": []byte("x")}, "readme.txt")
	if _, err := cdr.ParseZip(z); err == nil {
		t.Fatal("expected error when the zip has no XML")
	}
}

func TestParseZipInvalid(t *testing.T) {
	if _, err := cdr.ParseZip([]byte("not a zip")); err == nil {
		t.Fatal("expected error for invalid zip")
	}
}
