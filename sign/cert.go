package sign

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// Certificate is the issuer's signing key and X.509 certificate.
type Certificate struct {
	Key  *rsa.PrivateKey
	Cert *x509.Certificate
}

// LoadPFX reads a PKCS#12 (.pfx/.p12) bundle, the format SUNAT certificates
// are usually delivered in.
func LoadPFX(data []byte, password string) (Certificate, error) {
	key, cert, _, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return Certificate{}, fmt.Errorf("sign: decode pfx: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return Certificate{}, fmt.Errorf("sign: unsupported private key type %T, want RSA", key)
	}
	return pair(rsaKey, cert)
}

// LoadPEM reads a PEM bundle holding the private key (PKCS#1 or PKCS#8) and
// the certificate, the format Greenter uses.
func LoadPEM(data []byte) (Certificate, error) {
	var (
		key  *rsa.PrivateKey
		cert *x509.Certificate
	)
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			if cert != nil {
				continue // keep the leaf, ignore the chain
			}
			c, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return Certificate{}, fmt.Errorf("sign: parse certificate: %w", err)
			}
			cert = c
		case "RSA PRIVATE KEY":
			k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return Certificate{}, fmt.Errorf("sign: parse private key: %w", err)
			}
			key = k
		case "PRIVATE KEY":
			k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return Certificate{}, fmt.Errorf("sign: parse private key: %w", err)
			}
			rsaKey, ok := k.(*rsa.PrivateKey)
			if !ok {
				return Certificate{}, fmt.Errorf("sign: unsupported private key type %T, want RSA", k)
			}
			key = rsaKey
		}
	}
	if key == nil {
		return Certificate{}, errors.New("sign: pem has no private key")
	}
	if cert == nil {
		return Certificate{}, errors.New("sign: pem has no certificate")
	}
	return pair(key, cert)
}

func pair(key *rsa.PrivateKey, cert *x509.Certificate) (Certificate, error) {
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || !pub.Equal(&key.PublicKey) {
		return Certificate{}, errors.New("sign: private key does not match certificate")
	}
	return Certificate{Key: key, Cert: cert}, nil
}
