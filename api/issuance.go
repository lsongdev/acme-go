package api

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/lsongdev/acme-go/acme"
)

const (
	orderTimeout       = 10 * time.Minute
	propagationTimeout = 2 * time.Minute
	persistTimeout     = 10 * time.Second
)

// publicResolvers are queried to check that DNS-01 records are visible from
// outside before asking the CA to validate them.
var publicResolvers = []string{"1.1.1.1:53", "8.8.8.8:53"}

// lookupTXT is replaced in tests.
var lookupTXT = func(ctx context.Context, server, name string) ([]string, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "udp", server)
		},
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return resolver.LookupTXT(ctx, name)
}

// startIssuance generates a fresh key for certificate and runs the ACME flow
// in the background.
func (s *Service) startIssuance(ctx context.Context, certificate *Certificate) (*Issuance, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("service is shutting down")
	}
	s.workers.Add(1)
	s.mu.Unlock()

	issuance, err := s.createIssuance(ctx, certificate)
	if err != nil {
		s.workers.Done()
		return nil, err
	}
	jobCtx, cancel := context.WithCancel(s.ctx)
	job := &job{cancel: cancel, resume: make(chan bool)}
	s.mu.Lock()
	s.jobs[issuance.ID] = job
	s.mu.Unlock()

	go func() {
		defer s.workers.Done()
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.jobs, issuance.ID)
			s.mu.Unlock()
		}()
		s.run(jobCtx, job, issuance)
	}()
	public := *issuance
	return &public, nil
}

func (s *Service) createIssuance(ctx context.Context, certificate *Certificate) (*Issuance, error) {
	csrPEM, keyPEM, err := GenerateCSR(certificate.Domains)
	if err != nil {
		return nil, err
	}
	issuance := &Issuance{
		CertificateID: certificate.ID,
		AccountID:     certificate.AccountID,
		Domains:       certificate.Domains,
		ChallengeType: certificate.ChallengeType,
		Status:        IssuanceRunning,
		Step:          "queued",
		CSRPEM:        csrPEM,
		KeyPEM:        keyPEM,
	}
	if err := s.store.CreateIssuance(ctx, issuance); err != nil {
		return nil, err
	}
	return issuance, nil
}

// run executes one issuance and records its outcome, even when ctx was
// canceled by CancelIssuance or Close.
func (s *Service) run(ctx context.Context, job *job, issuance *Issuance) {
	log := s.log.With("issuance", issuance.ID, "certificate", issuance.CertificateID)
	log.Info("issuance started", "domains", issuance.Domains, "challenge", issuance.ChallengeType)
	err := s.issue(ctx, job, issuance)

	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	// The key now lives on the certificate, or is useless; drop the copy.
	issuance.CSRPEM, issuance.KeyPEM = "", ""
	if err == nil {
		issuance.Status, issuance.Step, issuance.Error = IssuanceSucceeded, "done", ""
		log.Info("issuance succeeded")
	} else {
		if ctx.Err() != nil {
			err = errors.New("canceled")
			if s.ctx.Err() != nil {
				err = errors.New("interrupted: the server stopped before this issuance finished")
			}
		}
		issuance.Status, issuance.Error = IssuanceFailed, err.Error()
		log.Warn("issuance failed", "step", issuance.Step, "error", err)
		if certificate, getErr := s.store.GetCertificate(persist, issuance.CertificateID); getErr == nil && certificate.Status == CertificatePending {
			certificate.Status = CertificateFailed
			_ = s.store.UpdateCertificate(persist, certificate)
		}
	}
	if err := s.store.UpdateIssuance(persist, issuance); err != nil {
		log.Error("save issuance outcome", "error", err)
	}
}

func (s *Service) setStep(ctx context.Context, issuance *Issuance, status, step string) error {
	issuance.Status, issuance.Step, issuance.Error = status, step, ""
	return s.store.UpdateIssuance(ctx, issuance)
}

// pendingChallenge is a challenge the CA still needs to validate.
type pendingChallenge struct {
	domain           string
	authorizationURL string
	challenge        acme.Challenge
}

func (s *Service) issue(ctx context.Context, job *job, issuance *Issuance) error {
	client, account, err := s.accountClient(ctx, issuance.AccountID)
	if err != nil {
		return err
	}
	if account.Status != "valid" {
		return fmt.Errorf("account %d is %s", account.ID, account.Status)
	}

	if err := s.setStep(ctx, issuance, IssuanceRunning, "creating-order"); err != nil {
		return err
	}
	identifiers := make([]acme.Identifier, 0, len(issuance.Domains))
	for _, domain := range issuance.Domains {
		identifiers = append(identifiers, acme.Identifier{Type: "dns", Value: domain})
	}
	orderURL, order, err := client.CreateOrder(ctx, &acme.OrderRequest{Identifiers: identifiers})
	if err != nil {
		return fmt.Errorf("create order: %w", err)
	}
	record := &Order{AccountID: account.ID, IssuanceID: issuance.ID, URL: orderURL, Domains: issuance.Domains, Status: order.Status}
	if err := s.store.CreateOrder(ctx, record); err != nil {
		return err
	}
	defer func() {
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
		defer cancel()
		_ = s.store.UpdateOrderStatus(persist, record.ID, record.Status)
	}()

	if err := s.setStep(ctx, issuance, IssuanceRunning, "authorizing"); err != nil {
		return err
	}
	var pending []pendingChallenge
	for _, authorizationURL := range order.Authorizations {
		authorization, err := client.GetAuthorization(ctx, authorizationURL)
		if err != nil {
			return fmt.Errorf("get authorization: %w", err)
		}
		if authorization.Status == acme.AuthorizationStatusValid {
			continue
		}
		domain := authorization.Identifier.Value
		if authorization.Wildcard {
			domain = "*." + domain
		}
		found := false
		for _, challenge := range authorization.Challenges {
			if challenge.Type == issuance.ChallengeType {
				pending = append(pending, pendingChallenge{domain: domain, authorizationURL: authorizationURL, challenge: challenge})
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s does not offer %s", domain, issuance.ChallengeType)
		}
	}

	if len(pending) > 0 {
		cleanup, err := s.present(ctx, client, job, issuance, pending)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			return err
		}
		if err := s.setStep(ctx, issuance, IssuanceRunning, "validating"); err != nil {
			return err
		}
		for _, p := range pending {
			if err := client.CompleteChallenge(ctx, p.challenge.URL); err != nil {
				return fmt.Errorf("respond to %s challenge for %s: %w", p.challenge.Type, p.domain, err)
			}
		}
		for _, p := range pending {
			validateCtx, cancel := context.WithTimeout(ctx, s.options.ValidationTimeout)
			_, err := client.WaitAuthorization(validateCtx, p.authorizationURL)
			cancel()
			if err != nil {
				return fmt.Errorf("validate %s: %w", p.domain, err)
			}
		}
	}

	if err := s.setStep(ctx, issuance, IssuanceRunning, "finalizing"); err != nil {
		return err
	}
	orderCtx, cancel := context.WithTimeout(ctx, orderTimeout)
	defer cancel()
	order, err = client.WaitOrder(orderCtx, orderURL)
	if err != nil {
		return fmt.Errorf("wait for order: %w", err)
	}
	record.Status = order.Status
	if order.Status == acme.OrderStatusReady {
		block, _ := pem.Decode([]byte(issuance.CSRPEM))
		if block == nil {
			return errors.New("issuance has no CSR")
		}
		if order, err = client.FinalizeOrder(orderCtx, order.Finalize, block.Bytes); err != nil {
			return fmt.Errorf("finalize order: %w", err)
		}
		record.Status = order.Status
		if order.Status != acme.OrderStatusValid {
			if order, err = client.WaitOrder(orderCtx, orderURL); err != nil {
				return fmt.Errorf("wait for certificate: %w", err)
			}
			record.Status = order.Status
		}
	}
	if order.Status != acme.OrderStatusValid || order.Certificate == "" {
		return fmt.Errorf("order is %s without a certificate", order.Status)
	}

	if err := s.setStep(ctx, issuance, IssuanceRunning, "downloading"); err != nil {
		return err
	}
	certPEM, err := client.GetCertificatePEM(ctx, order.Certificate)
	if err != nil {
		return fmt.Errorf("download certificate: %w", err)
	}
	leaf, err := VerifyIssuedCertificate(certPEM, issuance.KeyPEM, issuance.Domains)
	if err != nil {
		return err
	}

	// Once the certificate exists, save it even if the issuance was canceled
	// in the meantime.
	persist, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancelPersist()
	certificate, err := s.store.GetCertificate(persist, issuance.CertificateID)
	if err != nil {
		return err
	}
	certificate.CertPEM, certificate.KeyPEM = certPEM, issuance.KeyPEM
	certificate.Status = CertificateActive
	applyLeaf(certificate, leaf)
	return s.store.UpdateCertificate(persist, certificate)
}

// present makes the pending challenges answerable. The returned cleanup
// removes them again and is non-nil whenever something was presented.
func (s *Service) present(ctx context.Context, client *acme.Client, job *job, issuance *Issuance, pending []pendingChallenge) (func(), error) {
	if issuance.ChallengeType == ChallengeHTTP01 {
		var tokens []string
		cleanup := func() {
			for _, token := range tokens {
				s.deleteToken(token)
			}
		}
		for _, p := range pending {
			file, err := client.HTTP01KeyAuthorization(p.challenge.Token)
			if err != nil {
				return cleanup, err
			}
			s.putToken(p.challenge.Token, string(file.Content))
			tokens = append(tokens, p.challenge.Token)
		}
		return cleanup, nil
	}

	records := make([]DNSRecord, 0, len(pending))
	for _, p := range pending {
		txt, err := client.DNS01KeyAuthorization(p.challenge.Token)
		if err != nil {
			return nil, err
		}
		name := txt.Name + "." + strings.TrimPrefix(p.domain, "*.")
		records = append(records, DNSRecord{Domain: p.domain, Name: name, Value: txt.Content})
	}
	issuance.DNSRecords = records

	if provider := s.options.DNSProvider; provider != nil {
		var presented []DNSRecord
		cleanup := func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
			defer cancel()
			for _, record := range presented {
				if err := provider.CleanUp(cleanupCtx, record.Name, record.Value); err != nil {
					s.log.Warn("DNS-01 cleanup failed", "name", record.Name, "error", err)
				}
			}
		}
		if err := s.setStep(ctx, issuance, IssuanceRunning, "publishing-dns"); err != nil {
			return cleanup, err
		}
		for _, record := range records {
			if err := provider.Present(ctx, record.Name, record.Value); err != nil {
				return cleanup, fmt.Errorf("publish TXT record %s: %w", record.Name, err)
			}
			presented = append(presented, record)
		}
		if err := s.setStep(ctx, issuance, IssuanceRunning, "checking-dns"); err != nil {
			return cleanup, err
		}
		if err := waitDNS(ctx, records, propagationTimeout); err != nil {
			// Public resolvers may lag the authoritative servers the CA asks.
			s.log.Warn("DNS-01 records not yet visible, validating anyway", "issuance", issuance.ID, "error", err)
		}
		return cleanup, nil
	}

	// Manual DNS-01: wait until the operator has published the records.
	message := ""
	for {
		if err := s.setStep(ctx, issuance, IssuanceWaiting, "awaiting-dns"); err != nil {
			return nil, err
		}
		if message != "" {
			issuance.Error = message
			if err := s.store.UpdateIssuance(ctx, issuance); err != nil {
				return nil, err
			}
		}
		var force bool
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case force = <-job.resume:
		}
		if err := s.setStep(ctx, issuance, IssuanceRunning, "checking-dns"); err != nil {
			return nil, err
		}
		if force {
			return nil, nil
		}
		err := checkDNS(ctx, records)
		if err == nil {
			return nil, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message = err.Error()
	}
}

// checkDNS reports an error unless every record is visible through at least
// one public resolver.
func checkDNS(ctx context.Context, records []DNSRecord) error {
	var missing []string
	for _, record := range records {
		visible := false
		for _, server := range publicResolvers {
			values, err := lookupTXT(ctx, server, record.Name)
			if err == nil && slices.Contains(values, record.Value) {
				visible = true
				break
			}
		}
		if !visible {
			missing = append(missing, record.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("TXT records not visible in public DNS yet: %s", strings.Join(missing, ", "))
	}
	return nil
}

func waitDNS(ctx context.Context, records []DNSRecord, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err := checkDNS(ctx, records)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(5 * time.Second):
		}
	}
}
