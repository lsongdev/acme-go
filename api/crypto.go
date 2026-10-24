package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// NormalizeDomains lowercases, validates and de-duplicates DNS names while
// keeping their order; the first name becomes the certificate's common name.
func NormalizeDomains(values []string) ([]string, error) {
	domains := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if name == "" {
			continue
		}
		if err := validateDomain(name); err != nil {
			return nil, err
		}
		if !slices.Contains(domains, name) {
			domains = append(domains, name)
		}
	}
	if len(domains) == 0 {
		return nil, invalidf("at least one domain is required")
	}
	return domains, nil
}

func validateDomain(name string) error {
	base := strings.TrimPrefix(name, "*.")
	if len(base) > 253 || !strings.Contains(base, ".") {
		return invalidf("invalid domain name %q", name)
	}
	for _, label := range strings.Split(base, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return invalidf("invalid domain name %q", name)
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return invalidf("invalid domain name %q", name)
			}
		}
	}
	return nil
}

func hasWildcard(domains []string) bool {
	return slices.ContainsFunc(domains, func(domain string) bool {
		return strings.HasPrefix(domain, "*.")
	})
}

// GenerateCSR creates a new P-256 key and a CSR covering domains, both PEM
// encoded.
func GenerateCSR(domains []string) (csrPEM, keyPEM string, err error) {
	if len(domains) == 0 {
		return "", "", errors.New("at least one domain is required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}, key)
	if err != nil {
		return "", "", err
	}
	csrPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return csrPEM, keyPEM, nil
}

// VerifyIssuedCertificate checks that the issued chain matches keyPEM and
// that its leaf covers every requested domain.
func VerifyIssuedCertificate(certPEM, keyPEM string, domains []string) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("certificate does not match its private key: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	for _, domain := range domains {
		if !slices.ContainsFunc(leaf.DNSNames, func(name string) bool { return strings.EqualFold(name, domain) }) {
			return nil, fmt.Errorf("issued certificate does not contain %s", domain)
		}
	}
	return leaf, nil
}

// ParseLeaf returns the first certificate in a PEM chain.
func ParseLeaf(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no PEM certificate found")
	}
	return x509.ParseCertificate(block.Bytes)
}

// applyLeaf copies the display fields of the issued leaf onto certificate.
func applyLeaf(certificate *Certificate, leaf *x509.Certificate) {
	certificate.Issuer = leaf.Issuer.CommonName
	if certificate.Issuer == "" && len(leaf.Issuer.Organization) > 0 {
		certificate.Issuer = leaf.Issuer.Organization[0]
	}
	certificate.SerialNumber = fmt.Sprintf("%X", leaf.SerialNumber)
	certificate.NotBefore = leaf.NotBefore
	certificate.NotAfter = leaf.NotAfter
}
