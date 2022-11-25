package acme

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

type RevokeCertRequest struct {
	Certificate string `json:"certificate"`
	Reason      int    `json:"reason"`
}

func (client *Client) GetCertificatePEM(certUrl string) (cert string, err error) {
	_, body, err := client.get(certUrl)
	cert = string(body)
	return
}

func (client *Client) GetCertificate(certUrl string) (cert *x509.Certificate, err error) {
	body, err := client.GetCertificatePEM(certUrl)
	if err != nil {
		return
	}
	p, _ := pem.Decode([]byte(body))
	if p == nil {
		err = fmt.Errorf("no PEM data found")
		return
	}
	return x509.ParseCertificate(p.Bytes)
}

func (client *Client) RevokeCert(cert *x509.Certificate, reason int) (err error) {
	revokeReq := RevokeCertRequest{
		Certificate: base64.RawURLEncoding.EncodeToString(cert.Raw),
		Reason:      reason,
	}
	_, _, err = client.post(client.Directory.RevokeCert, revokeReq)
	return
}
