package api

import "context"

// Store persists providers, accounts, certificates, issuances and orders.
// Implementations return ErrNotFound for missing records and ErrConflict for
// uniqueness or reference violations.
type Store interface {
	ListProviders(ctx context.Context) ([]Provider, error)
	GetProvider(ctx context.Context, id int64) (*Provider, error)
	CreateProvider(ctx context.Context, provider *Provider) error
	// DeleteProvider fails with ErrConflict while accounts reference it.
	DeleteProvider(ctx context.Context, id int64) error

	ListAccounts(ctx context.Context) ([]Account, error)
	GetAccount(ctx context.Context, id int64) (*Account, error)
	CreateAccount(ctx context.Context, account *Account) error
	UpdateAccountStatus(ctx context.Context, id int64, status string) error
	// DeleteAccount fails with ErrConflict while certificates reference it.
	DeleteAccount(ctx context.Context, id int64) error

	ListCertificates(ctx context.Context) ([]Certificate, error)
	GetCertificate(ctx context.Context, id int64) (*Certificate, error)
	CreateCertificate(ctx context.Context, certificate *Certificate) error
	// UpdateCertificate saves every mutable field of certificate.
	UpdateCertificate(ctx context.Context, certificate *Certificate) error
	DeleteCertificate(ctx context.Context, id int64) error

	ListIssuances(ctx context.Context, certificateID int64) ([]Issuance, error)
	GetIssuance(ctx context.Context, id int64) (*Issuance, error)
	// CreateIssuance fails with ErrConflict if the certificate already has an
	// unfinished issuance.
	CreateIssuance(ctx context.Context, issuance *Issuance) error
	// UpdateIssuance saves status, step, error and DNS records.
	UpdateIssuance(ctx context.Context, issuance *Issuance) error
	// FailUnfinishedIssuances marks issuances left running by a previous
	// process as failed and returns how many were affected.
	FailUnfinishedIssuances(ctx context.Context, reason string) (int64, error)

	ListOrders(ctx context.Context) ([]Order, error)
	CreateOrder(ctx context.Context, order *Order) error
	UpdateOrderStatus(ctx context.Context, id int64, status string) error
}
