package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"
)

// TLSCertificates returns every active, unexpired certificate as a
// tls.Certificate ready to serve, for TLS servers that take their
// certificates from this service.
func (s *Service) TLSCertificates(ctx context.Context) ([]tls.Certificate, error) {
	certificates, err := s.store.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]tls.Certificate, 0, len(certificates))
	for _, c := range certificates {
		if c.Status != "active" || c.CertPEM == "" || c.KeyPEM == "" || now.After(c.NotAfter) {
			continue
		}
		pair, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		if err != nil {
			return nil, fmt.Errorf("certificate %d: %w", c.ID, err)
		}
		out = append(out, pair)
	}
	return out, nil
}
