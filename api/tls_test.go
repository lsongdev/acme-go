package api_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/lsongdev/acme-go/api"
)

// A server that owns port 80 itself answers challenges through
// KeyAuthorization and serves the results through TLSCertificates.
func TestCertificatesForExternalServers(t *testing.T) {
	e := newEnv(t)
	e.ca.fetchHTTP01 = func(_, token string) (string, bool) { return e.service.KeyAuthorization(token) }
	if _, ok := e.service.KeyAuthorization(""); ok {
		t.Fatal("empty token answered")
	}
	account := e.registerAccount()

	var result created
	e.call("POST", "/certificates", api.CreateCertificateRequest{
		Domains: []string{"example.com"}, AccountID: account.ID, ChallengeType: "http-01",
	}, 202, &result)
	if issuance := e.waitIssuance(result.Issuance.ID, api.IssuanceSucceeded, api.IssuanceFailed); issuance.Status != api.IssuanceSucceeded {
		t.Fatalf("issuance failed at %s: %s", issuance.Step, issuance.Error)
	}

	certs, err := e.service.TLSCertificates(context.Background())
	if err != nil || len(certs) != 1 || certs[0].Leaf == nil || certs[0].Leaf.DNSNames[0] != "example.com" {
		t.Fatalf("TLSCertificates = %d certs, %v", len(certs), err)
	}

	// Revoked certificates are no longer served.
	e.call("POST", fmt.Sprintf("/certificates/%d/revoke", result.Certificate.ID), map[string]int{"reason": 4}, 200, nil)
	if certs, _ := e.service.TLSCertificates(context.Background()); len(certs) != 0 {
		t.Fatalf("revoked certificate still served: %d", len(certs))
	}
}
