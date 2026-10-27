// Package sign applies the XMLDSig enveloped signature SUNAT requires.
//
// The signature is placed inside the first ext:ExtensionContent element and
// signs the whole document (Reference URI="") with inclusive Canonical XML 1.0.
// Defaults mirror Greenter: RSA-SHA1 with a SHA-1 digest.
package sign

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	_ "crypto/sha1" // register SHA-1 for crypto.SHA1
	_ "crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	nsDS          = "http://www.w3.org/2000/09/xmldsig#"
	nsExtension   = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
	algC14N       = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	algEnveloped  = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
	defaultSignID = "SUNATGO-SIGN"
)

// Algorithm pairs a signature method with its digest method.
type Algorithm struct {
	hash      crypto.Hash
	signature string
	digest    string
}

var (
	// SHA1 is RSA-SHA1 with a SHA-1 digest, Greenter's default.
	SHA1 = Algorithm{crypto.SHA1, "http://www.w3.org/2000/09/xmldsig#rsa-sha1", "http://www.w3.org/2000/09/xmldsig#sha1"}
	// SHA256 is RSA-SHA256 with a SHA-256 digest.
	SHA256 = Algorithm{crypto.SHA256, "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", "http://www.w3.org/2001/04/xmlenc#sha256"}
)

// Signer signs an XML document.
type Signer interface {
	Sign(doc []byte) ([]byte, error)
}

// XMLSigner signs SUNAT UBL documents with a certificate.
type XMLSigner struct {
	cert Certificate
	id   string
	alg  Algorithm
}

// Option configures an XMLSigner.
type Option func(*XMLSigner)

// WithID sets the ds:Signature Id. It must be an XML NCName and match the
// cac:Signature URI of the document; the ubl package uses "SUNATGO-SIGN".
// Sign rejects invalid IDs with an error starting with "sign: invalid signature id".
func WithID(id string) Option { return func(s *XMLSigner) { s.id = id } }

// WithAlgorithm selects the signature and digest algorithm.
func WithAlgorithm(a Algorithm) Option { return func(s *XMLSigner) { s.alg = a } }

// New returns a signer for the given certificate.
func New(cert Certificate, opts ...Option) *XMLSigner {
	s := &XMLSigner{cert: cert, id: defaultSignID, alg: SHA1}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Sign returns doc with a ds:Signature inserted into ext:ExtensionContent.
func (s *XMLSigner) Sign(doc []byte) ([]byte, error) {
	if s.cert.Key == nil || s.cert.Cert == nil {
		return nil, errors.New("sign: certificate and key are required")
	}
	if !validNCName(s.id) {
		return nil, fmt.Errorf("sign: invalid signature id %q", s.id)
	}
	d := etree.NewDocument()
	if err := d.ReadFromBytes(doc); err != nil {
		return nil, fmt.Errorf("sign: parse xml: %w", err)
	}
	root := d.Root()
	if root == nil {
		return nil, errors.New("sign: empty document")
	}
	if len(d.ChildElements()) != 1 {
		return nil, errors.New("sign: multiple root elements")
	}
	target := findExtensionContent(root)
	if target == nil {
		return nil, errors.New("sign: ext:ExtensionContent not found")
	}

	// Enveloped transform: the digest covers the document without the
	// signature, which is exactly the document before insertion.
	canonical, err := canonicalDocument(d)
	if err != nil {
		return nil, fmt.Errorf("sign: canonicalize document: %w", err)
	}
	digest, err := s.hash(canonical)
	if err != nil {
		return nil, err
	}

	signedInfo := s.signedInfo(base64.StdEncoding.EncodeToString(digest))
	// Inclusive C14N renders in-scope namespaces and inherited xml:*
	// attributes on the SignedInfo apex, with namespaces before attributes.
	ns := inScopeNamespaces(target)
	ns["ds"] = nsDS
	canonicalSI := "<ds:SignedInfo" + renderNamespaces(ns) + renderXMLAttributes(target) + ">" + signedInfo + "</ds:SignedInfo>"
	siDigest, err := s.hash([]byte(canonicalSI))
	if err != nil {
		return nil, err
	}
	sigValue, err := rsa.SignPKCS1v15(rand.Reader, s.cert.Key, s.alg.hash, siDigest)
	if err != nil {
		return nil, fmt.Errorf("sign: rsa: %w", err)
	}

	signature := `<ds:Signature xmlns:ds="` + nsDS + `" Id="` + escapeAttr(s.id) + `">` +
		"<ds:SignedInfo>" + signedInfo + "</ds:SignedInfo>" +
		"<ds:SignatureValue>" + base64.StdEncoding.EncodeToString(sigValue) + "</ds:SignatureValue>" +
		"<ds:KeyInfo><ds:X509Data><ds:X509Certificate>" +
		base64.StdEncoding.EncodeToString(s.cert.Cert.Raw) +
		"</ds:X509Certificate></ds:X509Data></ds:KeyInfo></ds:Signature>"

	frag := etree.NewDocument()
	if err := frag.ReadFromString(signature); err != nil {
		return nil, fmt.Errorf("sign: build signature: %w", err)
	}
	// No whitespace is added around the signature: the enveloped transform
	// removes only the element, so extra text would break the digest.
	target.AddChild(frag.Root())
	// Preserve referenced tabs, newlines and carriage returns in attributes.
	d.WriteSettings.CanonicalAttrVal = true
	out, err := d.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("sign: write xml: %w", err)
	}
	return out, nil
}

// signedInfo returns the canonical children of ds:SignedInfo.
func (s *XMLSigner) signedInfo(digest string) string {
	return `<ds:CanonicalizationMethod Algorithm="` + algC14N + `"></ds:CanonicalizationMethod>` +
		`<ds:SignatureMethod Algorithm="` + s.alg.signature + `"></ds:SignatureMethod>` +
		`<ds:Reference URI="">` +
		`<ds:Transforms><ds:Transform Algorithm="` + algEnveloped + `"></ds:Transform></ds:Transforms>` +
		`<ds:DigestMethod Algorithm="` + s.alg.digest + `"></ds:DigestMethod>` +
		`<ds:DigestValue>` + digest + `</ds:DigestValue>` +
		`</ds:Reference>`
}

func (s *XMLSigner) hash(b []byte) ([]byte, error) {
	if !s.alg.hash.Available() {
		return nil, fmt.Errorf("sign: hash %v unavailable", s.alg.hash)
	}
	h := s.alg.hash.New()
	h.Write(b)
	return h.Sum(nil), nil
}

// canonicalDocument includes top-level processing instructions, but neither
// the XML declaration nor comments. Whitespace outside the root is omitted.
func canonicalDocument(d *etree.Document) ([]byte, error) {
	root := d.Root().Copy()
	stripXMLNamespace(root)
	canonicalRoot, err := dsig.MakeC14N10RecCanonicalizer().Canonicalize(root)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	afterRoot := false
	for _, token := range d.Child {
		switch node := token.(type) {
		case *etree.Element:
			b.Write(canonicalRoot)
			afterRoot = true
		case *etree.ProcInst:
			if node.Target == "xml" {
				continue
			}
			if afterRoot {
				b.WriteByte('\n')
			}
			b.WriteString("<?" + node.Target)
			if node.Inst != "" {
				b.WriteByte(' ')
				b.WriteString(strings.ReplaceAll(node.Inst, "\r", "&#xD;"))
			}
			b.WriteString("?>")
			if !afterRoot {
				b.WriteByte('\n')
			}
		}
	}
	return []byte(b.String()), nil
}

// C14N never renders the predefined xml namespace, even when explicit.
// Only the digest's copy is changed; the output keeps the declarations.
func stripXMLNamespace(el *etree.Element) {
	el.RemoveAttr("xmlns:xml")
	for _, child := range el.ChildElements() {
		stripXMLNamespace(child)
	}
}

// renderXMLAttributes renders inherited xml:* attributes in C14N order.
// All have the same namespace URI, so their local names determine the order.
func renderXMLAttributes(el *etree.Element) string {
	attrs := map[string]string{}
	for e := el; e != nil; e = e.Parent() {
		for _, a := range e.Attr {
			if a.Space == "xml" {
				if _, seen := attrs[a.Key]; !seen {
					attrs[a.Key] = a.Value
				}
			}
		}
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(` xml:` + key + `="` + escapeAttr(attrs[key]) + `"`)
	}
	return b.String()
}

// validNCName implements XML 1.0 NameStartChar/NameChar without the colon.
func validNCName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	for i, r := range name {
		if ncNameStart(r) {
			continue
		}
		if i == 0 || !(r == '-' || r == '.' || r >= '0' && r <= '9' ||
			r == 0xB7 || r >= 0x300 && r <= 0x36F || r >= 0x203F && r <= 0x2040) {
			return false
		}
	}
	return true
}

func ncNameStart(r rune) bool {
	return r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' ||
		r >= 0xC0 && r <= 0xD6 || r >= 0xD8 && r <= 0xF6 ||
		r >= 0xF8 && r <= 0x2FF || r >= 0x370 && r <= 0x37D ||
		r >= 0x37F && r <= 0x1FFF || r >= 0x200C && r <= 0x200D ||
		r >= 0x2070 && r <= 0x218F || r >= 0x2C00 && r <= 0x2FEF ||
		r >= 0x3001 && r <= 0xD7FF || r >= 0xF900 && r <= 0xFDCF ||
		r >= 0xFDF0 && r <= 0xFFFD || r >= 0x10000 && r <= 0xEFFFF
}

func findExtensionContent(el *etree.Element) *etree.Element {
	if el.Tag == "ExtensionContent" && el.NamespaceURI() == nsExtension {
		return el
	}
	for _, c := range el.ChildElements() {
		if found := findExtensionContent(c); found != nil {
			return found
		}
	}
	return nil
}

// inScopeNamespaces maps prefix ("" for default) to URI for el and its ancestors.
func inScopeNamespaces(el *etree.Element) map[string]string {
	ns := map[string]string{}
	for e := el; e != nil; e = e.Parent() {
		for _, a := range e.Attr {
			var prefix string
			switch {
			case a.Space == "" && a.Key == "xmlns":
				prefix = ""
			case a.Space == "xmlns":
				prefix = a.Key
			default:
				continue
			}
			if _, seen := ns[prefix]; !seen {
				ns[prefix] = a.Value
			}
		}
	}
	if ns[""] == "" {
		delete(ns, "") // xmlns="" is not rendered on the apex
	}
	delete(ns, "xml") // the predefined xml namespace is never rendered
	return ns
}

// renderNamespaces renders declarations in C14N order: default first, then by prefix.
func renderNamespaces(ns map[string]string) string {
	prefixes := make([]string, 0, len(ns))
	for p := range ns {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	var b strings.Builder
	for _, p := range prefixes {
		if p == "" {
			b.WriteString(` xmlns="`)
		} else {
			b.WriteString(` xmlns:` + p + `="`)
		}
		b.WriteString(escapeAttr(ns[p]) + `"`)
	}
	return b.String()
}

// escapeAttr escapes an attribute value as Canonical XML does.
func escapeAttr(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", `"`, "&quot;",
		"\t", "&#x9;", "\n", "&#xA;", "\r", "&#xD;",
	).Replace(s)
}
