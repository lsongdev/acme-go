package acme

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

type RevokeCertRequest struct {
	Certificate string `json:"certificate"`
	Reason      int    `json:"reason"`
}

func (c *Client) GetCertificatePEM(ctx context.Context, certURL string) (string, error) {
	_, body, err := c.postAsGet(ctx, certURL)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) GetCertificate(ctx context.Context, certURL string) (*x509.Certificate, error) {
	body, err := c.GetCertificatePEM(ctx, certURL)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(body))
	if block == nil {
		return nil, fmt.Errorf("acme: no PEM certificate found")
	}
	return x509.ParseCertificate(block.Bytes)
}

func (c *Client) RevokeCert(ctx context.Context, cert *x509.Certificate, reason int) error {
	directory, err := c.directory(ctx)
	if err != nil {
		return err
	}
	_, _, err = c.post(ctx, directory.RevokeCert, RevokeCertRequest{
		Certificate: base64.RawURLEncoding.EncodeToString(cert.Raw),
		Reason:      reason,
	})
	return err
}
