// Package api manages ACME providers, accounts and certificates on top of the
// acme client, and exposes them as a JSON HTTP API.
package api

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when an operation conflicts with current state.
	ErrConflict = errors.New("conflict")
	// ErrInvalid is wrapped by errors caused by invalid client input.
	ErrInvalid = errors.New("invalid request")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func conflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

// Challenge types supported by the issuance runner.
const (
	ChallengeHTTP01 = "http-01"
	ChallengeDNS01  = "dns-01"
)

// Certificate statuses.
const (
	CertificatePending = "pending" // first issuance has not completed
	CertificateActive  = "active"
	CertificateFailed  = "failed" // first issuance failed; nothing usable
	CertificateRevoked = "revoked"
)

// Account statuses.
const (
	AccountValid       = "valid"
	AccountDeactivated = "deactivated"
)

// Issuance statuses.
const (
	IssuanceRunning   = "running"
	IssuanceWaiting   = "waiting" // waiting for the operator, see Issuance.DNSRecords
	IssuanceSucceeded = "succeeded"
	IssuanceFailed    = "failed"
)

// Provider is an ACME certificate authority identified by its directory URL.
type Provider struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	DirectoryURL string    `json:"directory_url"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProviderDirectory is the subset of an ACME directory shown to operators.
type ProviderDirectory struct {
	TermsOfService          string `json:"terms_of_service"`
	Website                 string `json:"website"`
	ExternalAccountRequired bool   `json:"external_account_required"`
}

// Account is an ACME account registered with a provider. KeyPEM never leaves
// the server.
type Account struct {
	ID           int64     `json:"id"`
	ProviderID   int64     `json:"provider_id"`
	ProviderName string    `json:"provider_name"`
	Email        string    `json:"email"`
	URL          string    `json:"url"`
	Status       string    `json:"status"`
	Thumbprint   string    `json:"thumbprint"`
	KeyPEM       string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Certificate is a managed certificate. It keeps the settings used to issue
// it so it can be renewed without operator input.
type Certificate struct {
	ID            int64     `json:"id"`
	Domains       []string  `json:"domains"`
	AccountID     int64     `json:"account_id"`
	ChallengeType string    `json:"challenge_type"`
	AutoRenew     bool      `json:"auto_renew"`
	Status        string    `json:"status"`
	CertPEM       string    `json:"cert_pem,omitempty"`
	KeyPEM        string    `json:"-"`
	Issuer        string    `json:"issuer,omitempty"`
	SerialNumber  string    `json:"serial_number,omitempty"`
	NotBefore     time.Time `json:"not_before,omitzero"`
	NotAfter      time.Time `json:"not_after,omitzero"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Order records an ACME order created by an issuance.
type Order struct {
	ID         int64     `json:"id"`
	AccountID  int64     `json:"account_id"`
	IssuanceID int64     `json:"issuance_id"`
	URL        string    `json:"url"`
	Domains    []string  `json:"domains"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DNSRecord is a TXT record that must be published for a DNS-01 challenge.
type DNSRecord struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

// Issuance is one run of the ACME flow for a certificate, either its first
// issuance or a renewal. Key material for the new certificate lives on the
// issuance until it succeeds, so a failed renewal leaves the current
// certificate untouched.
type Issuance struct {
	ID            int64       `json:"id"`
	CertificateID int64       `json:"certificate_id"`
	AccountID     int64       `json:"account_id"`
	Domains       []string    `json:"domains"`
	ChallengeType string      `json:"challenge_type"`
	Status        string      `json:"status"`
	Step          string      `json:"step"`
	Error         string      `json:"error,omitempty"`
	DNSRecords    []DNSRecord `json:"dns_records,omitempty"`
	CSRPEM        string      `json:"-"`
	KeyPEM        string      `json:"-"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// CreateProviderRequest adds an ACME provider.
type CreateProviderRequest struct {
	Name         string `json:"name"`
	DirectoryURL string `json:"directory_url"`
}

// RegisterAccountRequest registers a new account with a provider.
type RegisterAccountRequest struct {
	ProviderID int64  `json:"provider_id"`
	Email      string `json:"email"`
	AgreeTerms bool   `json:"agree_terms"`
}

// CreateCertificateRequest creates a certificate and starts its issuance.
type CreateCertificateRequest struct {
	Domains       []string `json:"domains"`
	AccountID     int64    `json:"account_id"`
	ChallengeType string   `json:"challenge_type"`
	AutoRenew     bool     `json:"auto_renew"`
}

// UpdateCertificateRequest changes how a certificate is renewed.
type UpdateCertificateRequest struct {
	AccountID     *int64  `json:"account_id,omitempty"`
	ChallengeType *string `json:"challenge_type,omitempty"`
	AutoRenew     *bool   `json:"auto_renew,omitempty"`
}
