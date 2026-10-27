package sign_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/villano14/sunat-go/internal/testcert"
	"github.com/villano14/sunat-go/sign"
)

const extensionNS = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"

func TestSignCanonicalizationWithXmlsec1(t *testing.T) {
	document := func(attrs, body string) string {
		return `<Invoice xmlns:ext="` + extensionNS + `"` + attrs + `>` + body + `</Invoice>`
	}
	const target = `<ext:ExtensionContent/>`
	tests := []struct {
		name string
		xml  string
		id   string
	}{
		{
			name: "default_prefixed_shadowed_namespaces",
			xml: document(` xmlns="urn:invoice" xmlns:p="urn:outer"`,
				`<p:Wrapper xmlns:p="urn:inner" xmlns:q="urn:unused">`+target+`</p:Wrapper>`),
		},
		{
			name: "default_namespace_reset",
			xml:  document(` xmlns="urn:invoice"`, `<ext:ExtensionContent xmlns=""/>`),
		},
		{
			name: "root_declares_ds",
			xml:  document(` xmlns:ds="http://www.w3.org/2000/09/xmldsig#"`, target),
		},
		{
			name: "explicit_xml_namespace",
			xml:  document(` xmlns:xml="http://www.w3.org/XML/1998/namespace"`, target),
		},
		{
			name: "nested_explicit_xml_namespace",
			xml:  document("", `<Wrapper xmlns:xml="http://www.w3.org/XML/1998/namespace">`+target+`</Wrapper>`),
		},
		{
			name: "inherited_xml_lang",
			xml:  document(` xml:lang="en"`, `<Wrapper xml:lang="es-PE">`+target+`</Wrapper>`),
		},
		{
			name: "inherited_xml_space",
			xml:  document(` xml:space="preserve"`, `<Wrapper xml:space="default">`+target+`</Wrapper>`),
		},
		{
			name: "inherited_xml_attributes_order_and_escaping",
			xml: document(` xml:space="preserve" xml:lang="en" xml:base="https://example.test/a?b=1&amp;c=2"`,
				`<ext:ExtensionContent xml:lang="" xml:custom="a&#x9;b&#xA;c&#xD;d&amp;&lt;&quot;"/>`),
		},
		{
			name: "processing_instructions_before_root",
			xml:  `<?xml version="1.0"?>` + "\n<?before data?>\n<?empty?>\n" + document("", target),
		},
		{
			name: "processing_instructions_after_root",
			xml:  document("", target) + "\n<?after data?>\n<?empty?>\n",
		},
		{
			name: "processing_instructions_and_comments",
			xml:  "<!--before--><?before a  b?>\n" + document("", target) + "\n<!--after--><?after?>",
		},
		{
			name: "comments_before_and_after_root",
			xml:  "<!--before-->\n" + document("", target) + "\n<!--after-->",
		},
		{
			name: "whitespace_inside_extension_content",
			xml:  document("", "<ext:ExtensionContent>\n \t </ext:ExtensionContent>"),
		},
		{
			name: "alternate_extension_prefix",
			xml:  `<Invoice xmlns:e="` + extensionNS + `"><e:ExtensionContent/></Invoice>`,
		},
		{
			name: "default_extension_namespace",
			xml:  `<Invoice><ExtensionContent xmlns="` + extensionNS + `"/></Invoice>`,
		},
		{
			name: "custom_id",
			xml:  document("", target),
			id:   "MY-SIGN",
		},
		{
			name: "ncname_punctuation",
			xml:  document("", target),
			id:   "_x.1",
		},
		{
			name: "ncname_unicode",
			xml:  document("", target),
			id:   "Firma_ñ",
		},
	}
	key, cert := testcert.New(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.id
			if id == "" {
				id = "SUNATGO-SIGN"
			}
			s := sign.New(sign.Certificate{Key: key, Cert: cert}, sign.WithAlgorithm(sign.SHA256), sign.WithID(id))
			input := []byte(tt.xml)
			before := bytes.Clone(input)
			signed, err := s.Sign(input)
			if !bytes.Equal(input, before) {
				t.Error("Sign mutated the input bytes")
			}
			if err != nil {
				t.Fatalf("Sign() error: %v", err)
			}
			if !bytes.Contains(signed, []byte(`Id="`+id+`"`)) {
				t.Errorf("signature Id = %q not preserved", id)
			}
			if err := xmlsec1Verify(t, signed, testcert.CertPEM(cert)); err != nil {
				t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, signed)
			}
		})
	}
}

func TestSignRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		xml    string
		id     string
		prefix string
	}{
		{name: "empty_document", prefix: "sign: empty document"},
		{name: "missing_extension_content", xml: `<Invoice/>`, prefix: "sign: ext:ExtensionContent not found"},
		{name: "unclosed_xml", xml: `<Invoice>`, prefix: "sign: parse xml:"},
		{name: "multiple_roots", xml: unsigned + `<Other/>`, prefix: "sign:"},
		{name: "wrong_namespace", xml: `<Invoice xmlns:ext="urn:wrong"><ext:ExtensionContent/></Invoice>`, prefix: "sign: ext:ExtensionContent not found"},
		{name: "no_namespace", xml: `<Invoice><ExtensionContent/></Invoice>`, prefix: "sign: ext:ExtensionContent not found"},
		{name: "invalid_id_empty", xml: unsigned, prefix: "sign: invalid signature id"},
		{name: "invalid_id_cr", xml: unsigned, id: "line1\rline2", prefix: "sign: invalid signature id"},
		{name: "invalid_id_markup", xml: unsigned, id: "a&<\"", prefix: "sign: invalid signature id"},
		{name: "invalid_id_digit_start", xml: unsigned, id: "1abc", prefix: "sign: invalid signature id"},
		{name: "invalid_id_space", xml: unsigned, id: "a b", prefix: "sign: invalid signature id"},
		{name: "invalid_id_colon", xml: unsigned, id: "a:b", prefix: "sign: invalid signature id"},
		{name: "invalid_id_dot_start", xml: unsigned, id: ".abc", prefix: "sign: invalid signature id"},
		{name: "invalid_id_combining_start", xml: unsigned, id: "\u0301abc", prefix: "sign: invalid signature id"},
		{name: "invalid_id_utf8", xml: unsigned, id: "a\xff", prefix: "sign: invalid signature id"},
	}
	key, cert := testcert.New(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := "SUNATGO-SIGN"
			if strings.HasPrefix(tt.name, "invalid_id_") {
				id = tt.id
			}
			s := sign.New(sign.Certificate{Key: key, Cert: cert}, sign.WithAlgorithm(sign.SHA256), sign.WithID(id))
			input := []byte(tt.xml)
			before := bytes.Clone(input)
			signed, err := s.Sign(input)
			if !bytes.Equal(input, before) {
				t.Error("Sign mutated the input bytes on rejection")
			}
			if err == nil {
				verifyErr := xmlsec1Verify(t, signed, testcert.CertPEM(cert))
				t.Fatalf("Sign() accepted invalid input; xmlsec1 result: %v", verifyErr)
			}
			if !strings.HasPrefix(err.Error(), tt.prefix) {
				t.Errorf("Sign() error = %q, want prefix %q", err, tt.prefix)
			}
			if signed != nil {
				t.Errorf("Sign() returned signed bytes on rejection: %q", signed)
			}
		})
	}
}
