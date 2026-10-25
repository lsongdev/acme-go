package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/acme-go/acme"
)

// DNSProvider publishes DNS-01 TXT records automatically. Without one,
// DNS-01 issuances wait for the operator to publish the records and call
// ContinueIssuance.
type DNSProvider interface {
	Present(ctx context.Context, fqdn, value string) error
	CleanUp(ctx context.Context, fqdn, value string) error
}

// DNSScope is implemented by a DNSProvider that can publish records for
// some names only, such as a DNS server hosting a few zones. An issuance
// with a record outside its scope falls back to manual DNS-01.
type DNSScope interface {
	Covers(fqdn string) bool
}

// Options configures a Service. Zero values select the defaults.
type Options struct {
	// HTTPClient is used for all ACME requests.
	HTTPClient *http.Client
	// DNSProvider publishes DNS-01 records; nil means manual DNS-01.
	DNSProvider DNSProvider
	// RenewBefore is how long before expiry auto-renewal starts (30 days).
	RenewBefore time.Duration
	// ValidationTimeout bounds waiting for each authorization (5 minutes).
	ValidationTimeout time.Duration
	Logger            *slog.Logger
}

// Service implements certificate management on top of the ACME client.
// Issuances run in background goroutines owned by the Service; call Close to
// stop them.
type Service struct {
	store   Store
	options Options
	log     *slog.Logger

	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup

	mu     sync.Mutex
	closed bool
	jobs   map[int64]*job    // unfinished issuances by issuance ID
	tokens map[string]string // HTTP-01 key authorizations by token
}

type job struct {
	cancel context.CancelFunc
	resume chan bool // receives force flag from ContinueIssuance
}

// NewService creates a Service. It marks issuances interrupted by a previous
// process as failed, since their in-memory challenge state is gone.
func NewService(ctx context.Context, store Store, options Options) (*Service, error) {
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if options.RenewBefore <= 0 {
		options.RenewBefore = 30 * 24 * time.Hour
	}
	if options.ValidationTimeout <= 0 {
		options.ValidationTimeout = 5 * time.Minute
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if _, err := store.FailUnfinishedIssuances(ctx, "interrupted: the server stopped before this issuance finished"); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &Service{
		store:   store,
		options: options,
		log:     options.Logger,
		ctx:     ctx,
		cancel:  cancel,
		jobs:    make(map[int64]*job),
		tokens:  make(map[string]string),
	}, nil
}

// Close cancels running issuances and waits for them to record their outcome.
func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.workers.Wait()
	return nil
}

// newClient returns an ACME client for the provider, optionally bound to an
// existing account.
func (s *Service) newClient(provider *Provider, account *Account) (*acme.Client, error) {
	config := &acme.Config{DirectoryURL: provider.DirectoryURL}
	if account != nil {
		config.AccountURL = account.URL
		config.AccountKey = account.KeyPEM
	}
	client, err := acme.NewClient(config)
	if err != nil {
		return nil, err
	}
	client.HTTPClient = s.options.HTTPClient
	return client, nil
}

func (s *Service) accountClient(ctx context.Context, accountID int64) (*acme.Client, *Account, error) {
	account, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	provider, err := s.store.GetProvider(ctx, account.ProviderID)
	if err != nil {
		return nil, nil, err
	}
	client, err := s.newClient(provider, account)
	return client, account, err
}

// Providers

func (s *Service) ListProviders(ctx context.Context) ([]Provider, error) {
	return s.store.ListProviders(ctx)
}

func (s *Service) CreateProvider(ctx context.Context, request CreateProviderRequest) (*Provider, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return nil, invalidf("name is required")
	}
	directory := strings.TrimSpace(request.DirectoryURL)
	if parsed, err := url.Parse(directory); err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, invalidf("directory_url must be an http(s) URL")
	}
	provider := &Provider{Name: name, DirectoryURL: directory}
	if err := s.store.CreateProvider(ctx, provider); err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *Service) DeleteProvider(ctx context.Context, id int64) error {
	return s.store.DeleteProvider(ctx, id)
}

// ProviderDirectory fetches the provider's ACME directory metadata.
func (s *Service) ProviderDirectory(ctx context.Context, id int64) (*ProviderDirectory, error) {
	provider, err := s.store.GetProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	client, err := s.newClient(provider, nil)
	if err != nil {
		return nil, err
	}
	directory, err := client.GetDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch ACME directory: %w", err)
	}
	return &ProviderDirectory{
		TermsOfService:          directory.Meta.TermsOfService,
		Website:                 directory.Meta.Website,
		ExternalAccountRequired: directory.Meta.ExternalAccountRequired,
	}, nil
}

// Accounts

func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	return s.store.ListAccounts(ctx)
}

func (s *Service) GetAccount(ctx context.Context, id int64) (*Account, error) {
	return s.store.GetAccount(ctx, id)
}

// RegisterAccount creates a new account key and registers it with the
// provider.
func (s *Service) RegisterAccount(ctx context.Context, request RegisterAccountRequest) (*Account, error) {
	provider, err := s.store.GetProvider(ctx, request.ProviderID)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(request.Email)
	if email != "" && !strings.Contains(email, "@") {
		return nil, invalidf("invalid email address %q", email)
	}
	client, err := s.newClient(provider, nil)
	if err != nil {
		return nil, err
	}
	directory, err := client.GetDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch ACME directory: %w", err)
	}
	if directory.Meta.ExternalAccountRequired {
		return nil, invalidf("%s requires external account binding, which is not supported", provider.Name)
	}
	if directory.Meta.TermsOfService != "" && !request.AgreeTerms {
		return nil, invalidf("the terms of service must be accepted")
	}
	if err := client.GenerateKey(); err != nil {
		return nil, err
	}
	keyPEM, err := client.ExportKey()
	if err != nil {
		return nil, err
	}
	thumbprint, err := client.GetThumbprint()
	if err != nil {
		return nil, err
	}
	accountRequest := &acme.AccountRequest{TermsOfServiceAgreed: request.AgreeTerms, Contact: []string{}}
	if email != "" {
		accountRequest.Contact = []string{"mailto:" + email}
	}
	accountURL, response, err := client.Register(ctx, accountRequest)
	if err != nil {
		return nil, fmt.Errorf("register account: %w", err)
	}
	account := &Account{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		Email:        email,
		URL:          accountURL,
		Status:       response.Status,
		Thumbprint:   thumbprint,
		KeyPEM:       keyPEM,
	}
	if account.Status == "" {
		account.Status = AccountValid
	}
	if err := s.store.CreateAccount(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// DeactivateAccount permanently deactivates the account at the provider.
func (s *Service) DeactivateAccount(ctx context.Context, id int64) (*Account, error) {
	client, account, err := s.accountClient(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Status == AccountDeactivated {
		return account, nil
	}
	if err := client.DeactivateAccount(ctx, account.URL); err != nil {
		return nil, fmt.Errorf("deactivate account: %w", err)
	}
	if err := s.store.UpdateAccountStatus(ctx, id, AccountDeactivated); err != nil {
		return nil, err
	}
	account.Status = AccountDeactivated
	return account, nil
}

// DeleteAccount removes the local account record and its key. The account
// stays registered at the provider.
func (s *Service) DeleteAccount(ctx context.Context, id int64) error {
	return s.store.DeleteAccount(ctx, id)
}

// Certificates

func (s *Service) ListCertificates(ctx context.Context) ([]Certificate, error) {
	return s.store.ListCertificates(ctx)
}

func (s *Service) GetCertificate(ctx context.Context, id int64) (*Certificate, error) {
	return s.store.GetCertificate(ctx, id)
}

func (s *Service) validateIssuance(ctx context.Context, accountID int64, challengeType string, domains []string) error {
	switch challengeType {
	case ChallengeHTTP01:
		if hasWildcard(domains) {
			return invalidf("wildcard domains require dns-01")
		}
	case ChallengeDNS01:
	default:
		return invalidf("challenge_type must be http-01 or dns-01")
	}
	account, err := s.store.GetAccount(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return invalidf("account %d does not exist", accountID)
	}
	if err != nil {
		return err
	}
	if account.Status != AccountValid {
		return invalidf("account %d is %s", accountID, account.Status)
	}
	return nil
}

// CreateCertificate records a new certificate and starts issuing it.
func (s *Service) CreateCertificate(ctx context.Context, request CreateCertificateRequest) (*Certificate, *Issuance, error) {
	domains, err := NormalizeDomains(request.Domains)
	if err != nil {
		return nil, nil, err
	}
	if err := s.validateIssuance(ctx, request.AccountID, request.ChallengeType, domains); err != nil {
		return nil, nil, err
	}
	certificate := &Certificate{
		Domains:       domains,
		AccountID:     request.AccountID,
		ChallengeType: request.ChallengeType,
		AutoRenew:     request.AutoRenew,
		Status:        CertificatePending,
	}
	if err := s.store.CreateCertificate(ctx, certificate); err != nil {
		return nil, nil, err
	}
	issuance, err := s.startIssuance(ctx, certificate)
	if err != nil {
		_ = s.store.DeleteCertificate(context.WithoutCancel(ctx), certificate.ID)
		return nil, nil, err
	}
	return certificate, issuance, nil
}

// UpdateCertificate changes renewal settings; they apply to the next issuance.
func (s *Service) UpdateCertificate(ctx context.Context, id int64, request UpdateCertificateRequest) (*Certificate, error) {
	certificate, err := s.store.GetCertificate(ctx, id)
	if err != nil {
		return nil, err
	}
	if request.AccountID != nil {
		certificate.AccountID = *request.AccountID
	}
	if request.ChallengeType != nil {
		certificate.ChallengeType = *request.ChallengeType
	}
	if request.AutoRenew != nil {
		certificate.AutoRenew = *request.AutoRenew
	}
	if err := s.validateIssuance(ctx, certificate.AccountID, certificate.ChallengeType, certificate.Domains); err != nil {
		return nil, err
	}
	if err := s.store.UpdateCertificate(ctx, certificate); err != nil {
		return nil, err
	}
	return certificate, nil
}

// RenewCertificate starts a new issuance for an existing certificate. It is
// also used to retry a failed first issuance.
func (s *Service) RenewCertificate(ctx context.Context, id int64) (*Issuance, error) {
	certificate, err := s.store.GetCertificate(ctx, id)
	if err != nil {
		return nil, err
	}
	if certificate.Status == CertificateRevoked {
		return nil, invalidf("a revoked certificate cannot be renewed")
	}
	if err := s.validateIssuance(ctx, certificate.AccountID, certificate.ChallengeType, certificate.Domains); err != nil {
		return nil, err
	}
	return s.startIssuance(ctx, certificate)
}

// RevokeCertificate revokes the current certificate with the given RFC 5280
// reason code and disables auto-renewal.
func (s *Service) RevokeCertificate(ctx context.Context, id int64, reason int) (*Certificate, error) {
	certificate, err := s.store.GetCertificate(ctx, id)
	if err != nil {
		return nil, err
	}
	if certificate.Status != CertificateActive {
		return nil, invalidf("only an active certificate can be revoked")
	}
	if reason < 0 || reason > 10 || reason == 7 {
		return nil, invalidf("invalid revocation reason %d", reason)
	}
	leaf, err := ParseLeaf(certificate.CertPEM)
	if err != nil {
		return nil, err
	}
	client, _, err := s.accountClient(ctx, certificate.AccountID)
	if err != nil {
		return nil, err
	}
	if err := client.RevokeCert(ctx, leaf, reason); err != nil {
		return nil, fmt.Errorf("revoke certificate: %w", err)
	}
	certificate.Status = CertificateRevoked
	certificate.AutoRenew = false
	if err := s.store.UpdateCertificate(ctx, certificate); err != nil {
		return nil, err
	}
	return certificate, nil
}

// DeleteCertificate removes a certificate, its key and its issuance history.
func (s *Service) DeleteCertificate(ctx context.Context, id int64) error {
	issuances, err := s.store.ListIssuances(ctx, id)
	if err != nil {
		return err
	}
	for _, issuance := range issuances {
		if !issuance.Finished() {
			return conflictf("certificate %d has an unfinished issuance", id)
		}
	}
	return s.store.DeleteCertificate(ctx, id)
}

// Issuances

// Finished reports whether the issuance has reached a final status.
func (i *Issuance) Finished() bool {
	return i.Status == IssuanceSucceeded || i.Status == IssuanceFailed
}

func (s *Service) ListIssuances(ctx context.Context, certificateID int64) ([]Issuance, error) {
	return s.store.ListIssuances(ctx, certificateID)
}

func (s *Service) GetIssuance(ctx context.Context, id int64) (*Issuance, error) {
	return s.store.GetIssuance(ctx, id)
}

// ContinueIssuance resumes a DNS-01 issuance waiting for the operator to
// publish its TXT records. Unless force is set, the records are first looked
// up through public resolvers.
func (s *Service) ContinueIssuance(ctx context.Context, id int64, force bool) error {
	issuance, err := s.store.GetIssuance(ctx, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	job := s.jobs[id]
	s.mu.Unlock()
	if issuance.Status != IssuanceWaiting || job == nil {
		return conflictf("issuance %d is not waiting for DNS records", id)
	}
	select {
	case job.resume <- force:
		return nil
	default:
		return conflictf("issuance %d is already continuing", id)
	}
}

// CancelIssuance stops an unfinished issuance; it is recorded as failed.
func (s *Service) CancelIssuance(ctx context.Context, id int64) error {
	if _, err := s.store.GetIssuance(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	job := s.jobs[id]
	s.mu.Unlock()
	if job == nil {
		return conflictf("issuance %d is not running", id)
	}
	job.cancel()
	return nil
}

// Orders

func (s *Service) ListOrders(ctx context.Context) ([]Order, error) {
	return s.store.ListOrders(ctx)
}

// HTTP-01

// HTTP01Handler serves key authorizations for pending HTTP-01 challenges at
// /.well-known/acme-challenge/{token}.
func (s *Service) HTTP01Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
		s.mu.Lock()
		value, ok := s.tokens[token]
		s.mu.Unlock()
		if !ok || token == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(value))
	})
}

// KeyAuthorization returns the key authorization for a pending HTTP-01
// challenge token, for servers that answer challenges themselves (such as a
// reverse proxy owning port 80) instead of mounting HTTP01Handler.
func (s *Service) KeyAuthorization(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.tokens[token]
	return value, ok && token != ""
}

func (s *Service) putToken(token, value string) {
	s.mu.Lock()
	s.tokens[token] = value
	s.mu.Unlock()
}

func (s *Service) deleteToken(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// Auto-renewal

// RenewDue starts renewals for active, auto-renewing certificates that expire
// within Options.RenewBefore. It returns the issuances it started.
func (s *Service) RenewDue(ctx context.Context) ([]Issuance, error) {
	certificates, err := s.store.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}
	var started []Issuance
	for _, certificate := range certificates {
		if !certificate.AutoRenew || certificate.Status != CertificateActive ||
			time.Until(certificate.NotAfter) > s.options.RenewBefore {
			continue
		}
		issuance, err := s.RenewCertificate(ctx, certificate.ID)
		if errors.Is(err, ErrConflict) {
			continue // already renewing
		}
		if err != nil {
			s.log.Warn("auto-renewal not started", "certificate", certificate.ID, "error", err)
			continue
		}
		s.log.Info("auto-renewal started", "certificate", certificate.ID, "issuance", issuance.ID)
		started = append(started, *issuance)
	}
	return started, nil
}

// RunAutoRenew calls RenewDue now and then every interval until ctx or the
// Service is done.
func (s *Service) RunAutoRenew(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.RenewDue(ctx); err != nil {
			s.log.Error("auto-renewal check failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
