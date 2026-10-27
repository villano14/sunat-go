package sign_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/villano14/sunat-go/internal/testcert"
	"github.com/villano14/sunat-go/sign"
)

// unsigned is a minimal SUNAT-shaped document: default namespace, prefixed
// children, and an empty ext:ExtensionContent nested below the root.
const unsigned = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2">
  <ext:UBLExtensions>
    <ext:UBLExtension>
      <ext:ExtensionContent/>
    </ext:UBLExtension>
  </ext:UBLExtensions>
  <cbc:ID>F001-1</cbc:ID>
  <cbc:Note>A &amp; B &lt;C&gt;</cbc:Note>
  <cbc:PayableAmount currencyID="PEN">236.00</cbc:PayableAmount>
</Invoice>
`

func newSigner(t *testing.T, alg sign.Algorithm) (*sign.XMLSigner, []byte) {
	t.Helper()
	key, cert := testcert.New(t)
	return sign.New(sign.Certificate{Key: key, Cert: cert}, sign.WithAlgorithm(alg)), testcert.CertPEM(cert)
}

// xmlsec1Verify verifies the signature with xmlsec1, an independent implementation.
func xmlsec1Verify(t *testing.T, signed, certPEM []byte) error {
	t.Helper()
	bin, err := exec.LookPath("xmlsec1")
	if err != nil {
		t.Skip("xmlsec1 not installed")
	}
	dir := t.TempDir()
	doc, crt := filepath.Join(dir, "signed.xml"), filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(doc, signed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crt, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--verify", "--pubkey-cert-pem", crt, doc)
	// Fedora/RHEL crypto policies reject RSA-SHA1 in OpenSSL; this variable
	// re-enables it for the verifier process only. Other systems ignore it.
	cmd.Env = append(os.Environ(), "OPENSSL_ENABLE_SHA1_SIGNATURES=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("xmlsec1: %s", out)
	}
	return err
}

func TestSignVerifiesWithXmlsec1(t *testing.T) {
	for name, alg := range map[string]sign.Algorithm{"sha256": sign.SHA256, "sha1": sign.SHA1} {
		t.Run(name, func(t *testing.T) {
			s, certPEM := newSigner(t, alg)
			signed, err := s.Sign([]byte(unsigned))
			if err != nil {
				t.Fatalf("Sign() error: %v", err)
			}
			if err := xmlsec1Verify(t, signed, certPEM); err != nil {
				t.Fatalf("xmlsec1 rejected the signature: %v\n%s", err, signed)
			}
		})
	}
}

func TestSignDetectsTampering(t *testing.T) {
	s, certPEM := newSigner(t, sign.SHA256)
	signed, err := s.Sign([]byte(unsigned))
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}
	tampered := bytes.Replace(signed, []byte("236.00"), []byte("1.00"), 1)
	if err := xmlsec1Verify(t, tampered, certPEM); err == nil {
		t.Fatal("xmlsec1 accepted a tampered document")
	}
}

func TestSignPlacement(t *testing.T) {
	s, _ := newSigner(t, sign.SHA1)
	signed, err := s.Sign([]byte(unsigned))
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}
	str := string(signed)
	start := strings.Index(str, "<ext:ExtensionContent>")
	sig := strings.Index(str, `<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Id="SUNATGO-SIGN">`)
	end := strings.Index(str, "</ext:ExtensionContent>")
	if start < 0 || sig < 0 || end < 0 || !(start < sig && sig < end) {
		t.Fatalf("ds:Signature with Id must be inside ext:ExtensionContent:\n%s", signed)
	}
	for _, s := range []string{
		`Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"`,
		`Algorithm="http://www.w3.org/2000/09/xmldsig#rsa-sha1"`,
		`Algorithm="http://www.w3.org/2000/09/xmldsig#sha1"`,
		`<ds:Reference URI="">`,
		`Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"`,
		"<ds:X509Certificate>",
	} {
		if !strings.Contains(str, s) {
			t.Errorf("missing %s", s)
		}
	}
}

func TestSignDefaultsToSHA1(t *testing.T) {
	key, cert := testcert.New(t)
	signed, err := sign.New(sign.Certificate{Key: key, Cert: cert}).Sign([]byte(unsigned))
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}
	if !bytes.Contains(signed, []byte("xmldsig#rsa-sha1")) {
		t.Error("default algorithm must be RSA-SHA1, as Greenter")
	}
}

func TestSignCustomID(t *testing.T) {
	key, cert := testcert.New(t)
	signed, err := sign.New(sign.Certificate{Key: key, Cert: cert}, sign.WithID("MY-SIGN")).Sign([]byte(unsigned))
	if err != nil {
		t.Fatalf("Sign() error: %v", err)
	}
	if !bytes.Contains(signed, []byte(`Id="MY-SIGN"`)) {
		t.Error("custom signature Id not applied")
	}
}

func TestSignWithoutExtensionContent(t *testing.T) {
	s, _ := newSigner(t, sign.SHA1)
	_, err := s.Sign([]byte(`<Invoice xmlns="urn:x"><ID>1</ID></Invoice>`))
	if err == nil {
		t.Fatal("expected error when ext:ExtensionContent is missing")
	}
}

func TestSignMalformedXML(t *testing.T) {
	s, _ := newSigner(t, sign.SHA1)
	if _, err := s.Sign([]byte("<Invoice>")); err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestLoadPEM(t *testing.T) {
	key, cert := testcert.New(t)
	bundle := append(testcert.KeyPEM(t, key), testcert.CertPEM(cert)...)
	got, err := sign.LoadPEM(bundle)
	if err != nil {
		t.Fatalf("LoadPEM() error: %v", err)
	}
	if !got.Cert.Equal(cert) || !got.Key.Equal(key) {
		t.Error("LoadPEM returned a different key or certificate")
	}
}

func TestLoadPEMMissingKey(t *testing.T) {
	_, cert := testcert.New(t)
	if _, err := sign.LoadPEM(testcert.CertPEM(cert)); err == nil {
		t.Fatal("expected error when the private key is missing")
	}
}

func TestLoadPEMMismatchedPair(t *testing.T) {
	key, _ := testcert.New(t)
	_, other := testcert.New(t)
	bundle := append(testcert.KeyPEM(t, key), testcert.CertPEM(other)...)
	if _, err := sign.LoadPEM(bundle); err == nil {
		t.Fatal("expected error when key and certificate do not match")
	}
}

func TestLoadPFX(t *testing.T) {
	key, cert := testcert.New(t)
	pfx, err := pkcs12.Modern.Encode(key, cert, nil, "secret")
	if err != nil {
		t.Fatal(err)
	}
	got, err := sign.LoadPFX(pfx, "secret")
	if err != nil {
		t.Fatalf("LoadPFX() error: %v", err)
	}
	if !got.Cert.Equal(cert) || !got.Key.Equal(key) {
		t.Error("LoadPFX returned a different key or certificate")
	}
	if _, err := sign.LoadPFX(pfx, "wrong"); err == nil {
		t.Error("expected error for wrong password")
	}
}
