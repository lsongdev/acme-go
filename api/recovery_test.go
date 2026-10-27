package api_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lsongdev/acme-go/acme"
	"github.com/lsongdev/acme-go/api"
)

type recoveryTransport struct {
	blockDownload     atomic.Bool
	holdDownload      atomic.Bool
	rateLimitDownload atomic.Bool
	loseFinalize      atomic.Bool
	orders            atomic.Int32
	finalizes         atomic.Int32
	downloads         atomic.Int32
}

func (tr *recoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	path := r.URL.Path
	if path == "/new-order" {
		tr.orders.Add(1)
	}
	if len(path) >= 10 && path[:10] == "/finalize/" {
		tr.finalizes.Add(1)
	}
	if len(path) >= 6 && path[:6] == "/cert/" {
		tr.downloads.Add(1)
		if tr.holdDownload.Load() {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		if tr.rateLimitDownload.Load() {
			return &http.Response{StatusCode: 429, Status: "429 Too Many Requests", Header: http.Header{"Retry-After": []string{"2"}}, Body: http.NoBody, Request: r}, nil
		}
		if tr.blockDownload.Load() {
			return nil, &net.DNSError{Err: "simulated timeout", IsTimeout: true}
		}
	}
	response, err := http.DefaultTransport.RoundTrip(r)
	if err == nil && len(path) >= 10 && path[:10] == "/finalize/" && tr.loseFinalize.Swap(false) {
		response.Body.Close()
		return nil, &net.DNSError{Err: "finalize response lost", IsTimeout: true}
	}
	return response, err
}

func recoveryEnv(t *testing.T, delay time.Duration, attempts int) (*env, *recoveryTransport, api.Account) {
	t.Helper()
	e := newEnv(t)
	account := e.registerAccount()
	e.service.Close()
	tr := &recoveryTransport{}
	e.options = api.Options{
		HTTPClient:    &http.Client{Transport: tr, Timeout: time.Second},
		RequestRetry:  acme.RetryPolicy{MaxAttempts: 1, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond},
		IssuanceRetry: acme.RetryPolicy{MaxAttempts: attempts, InitialDelay: delay, MaxDelay: delay},
	}
	e.newService()
	return e, tr, account
}

func createRecoveryCertificate(e *env, account api.Account) created {
	e.t.Helper()
	var result created
	e.call("POST", "/certificates", api.CreateCertificateRequest{Domains: []string{"recover.example.org"}, AccountID: account.ID, ChallengeType: "http-01"}, 202, &result)
	return result
}

func TestLostFinalizeResponseDoesNotCreateAnotherOrder(t *testing.T) {
	e, tr, account := recoveryEnv(t, time.Millisecond, 4)
	tr.loseFinalize.Store(true)
	result := createRecoveryCertificate(e, account)
	i := e.waitIssuance(result.Issuance.ID, api.IssuanceSucceeded, api.IssuanceFailed)
	if i.Status != api.IssuanceSucceeded || tr.orders.Load() != 1 || tr.finalizes.Load() != 1 {
		t.Fatalf("status=%s error=%s new orders=%d finalize=%d", i.Status, i.Error, tr.orders.Load(), tr.finalizes.Load())
	}
}

func TestDownloadRecoverySurvivesRestart(t *testing.T) {
	e, tr, account := recoveryEnv(t, 300*time.Millisecond, 4)
	tr.blockDownload.Store(true)
	result := createRecoveryCertificate(e, account)
	waiting := e.waitIssuance(result.Issuance.ID, api.IssuanceRetrying)
	saved, err := e.db.GetIssuance(context.Background(), waiting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.KeyPEM == "" || saved.CSRPEM == "" || saved.OrderURL == "" || saved.NextAttemptAt.IsZero() || saved.Attempts != 1 {
		t.Fatalf("incomplete checkpoint: %+v", waiting)
	}
	e.service.Close()
	stopped, _ := e.db.GetIssuance(context.Background(), saved.ID)
	if stopped.Status != api.IssuanceRetrying || stopped.KeyPEM != saved.KeyPEM || !stopped.NextAttemptAt.Equal(saved.NextAttemptAt) {
		t.Fatal("shutdown lost retry state")
	}
	tr.blockDownload.Store(false)
	e.newService()
	i := e.waitIssuance(saved.ID, api.IssuanceSucceeded, api.IssuanceFailed)
	certificate, _ := e.db.GetCertificate(context.Background(), result.Certificate.ID)
	if i.Status != api.IssuanceSucceeded || i.Attempts != 2 || certificate.KeyPEM != saved.KeyPEM || tr.orders.Load() != 1 || tr.finalizes.Load() != 1 {
		t.Fatalf("status=%s error=%s attempts=%d orders=%d finalize=%d", i.Status, i.Error, i.Attempts, tr.orders.Load(), tr.finalizes.Load())
	}
	done, _ := e.db.GetIssuance(context.Background(), saved.ID)
	if done.KeyPEM != "" || done.CSRPEM != "" {
		t.Fatal("successful issuance retained temporary key material")
	}
}

func TestCancelScheduledRetry(t *testing.T) {
	e, tr, account := recoveryEnv(t, 200*time.Millisecond, 4)
	tr.blockDownload.Store(true)
	result := createRecoveryCertificate(e, account)
	waiting := e.waitIssuance(result.Issuance.ID, api.IssuanceRetrying)
	calls := tr.downloads.Load()
	e.call("POST", fmt.Sprintf("/issuances/%d/cancel", waiting.ID), nil, 202, nil)
	i := e.waitIssuance(waiting.ID, api.IssuanceFailed)
	time.Sleep(250 * time.Millisecond)
	saved, _ := e.db.GetIssuance(context.Background(), waiting.ID)
	if i.Error != "canceled" || saved.KeyPEM != "" || !saved.NextAttemptAt.IsZero() || tr.downloads.Load() != calls {
		t.Fatalf("canceled job kept running: %+v", i)
	}
}

func TestManualRetryAfterAutomaticBudgetExhausted(t *testing.T) {
	e, tr, account := recoveryEnv(t, time.Millisecond, 2)
	tr.blockDownload.Store(true)
	result := createRecoveryCertificate(e, account)
	failed := e.waitIssuance(result.Issuance.ID, api.IssuanceFailed)
	saved, _ := e.db.GetIssuance(context.Background(), failed.ID)
	if failed.Attempts != 2 || saved.KeyPEM == "" || saved.OrderURL == "" {
		t.Fatalf("lost exhausted job: %+v", failed)
	}
	tr.blockDownload.Store(false)
	var retry api.Issuance
	e.call("POST", fmt.Sprintf("/certificates/%d/renew", result.Certificate.ID), nil, 202, &retry)
	i := e.waitIssuance(retry.ID, api.IssuanceSucceeded, api.IssuanceFailed)
	certificate, _ := e.db.GetCertificate(context.Background(), result.Certificate.ID)
	if i.Status != api.IssuanceSucceeded || certificate.KeyPEM != saved.KeyPEM || tr.orders.Load() != 1 || tr.finalizes.Load() != 1 {
		t.Fatalf("manual retry lost original order: %+v", i)
	}
}

func TestShutdownDuringDownloadResumesOriginalKey(t *testing.T) {
	e, tr, account := recoveryEnv(t, time.Millisecond, 4)
	tr.holdDownload.Store(true)
	result := createRecoveryCertificate(e, account)
	deadline := time.Now().Add(time.Second)
	for tr.downloads.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tr.downloads.Load() == 0 {
		t.Fatal("download never started")
	}
	e.service.Close()
	saved, err := e.db.GetIssuance(context.Background(), result.Issuance.ID)
	if err != nil || saved.Status != api.IssuanceRunning || saved.KeyPEM == "" {
		t.Fatal("running issuance was not checkpointed")
	}
	tr.holdDownload.Store(false)
	e.newService()
	i := e.waitIssuance(saved.ID, api.IssuanceSucceeded, api.IssuanceFailed)
	certificate, _ := e.db.GetCertificate(context.Background(), result.Certificate.ID)
	if i.Status != api.IssuanceSucceeded || i.Attempts != 1 || certificate.KeyPEM != saved.KeyPEM || tr.orders.Load() != 1 || tr.finalizes.Load() != 1 {
		t.Fatalf("resume failed: %+v", i)
	}
}

func TestRateLimitSurvivesStageDeadline(t *testing.T) {
	e, tr, account := recoveryEnv(t, time.Millisecond, 4)
	e.service.Close()
	e.options.OrderTimeout = 30 * time.Millisecond
	e.options.RequestRetry.MaxAttempts = 4
	e.newService()
	tr.rateLimitDownload.Store(true)
	result := createRecoveryCertificate(e, account)
	i := e.waitIssuance(result.Issuance.ID, api.IssuanceRetrying, api.IssuanceFailed)
	if i.Status != api.IssuanceRetrying || time.Until(i.NextAttemptAt) < time.Second {
		t.Fatalf("lost rate limit delay: %+v", i)
	}
	e.call("POST", fmt.Sprintf("/issuances/%d/cancel", i.ID), nil, 202, nil)
	e.waitIssuance(i.ID, api.IssuanceFailed)
}

func TestPermanentFailureIsNotRetryable(t *testing.T) {
	if acme.IsRetryable(errors.New("invalid CSR")) || acme.IsRetryable(&acme.Response{Status: 403, Type: "unauthorized"}) || acme.IsRetryable(context.Canceled) {
		t.Fatal("permanent failure was classified as transient")
	}
	e := newEnv(t)
	account := e.registerAccount()
	// Force a real CA challenge rejection rather than a transport failure.
	var result created
	e.call("POST", "/certificates", api.CreateCertificateRequest{Domains: []string{"reject.example.org"}, AccountID: account.ID, ChallengeType: "dns-01"}, 202, &result)
	waiting := e.waitIssuance(result.Issuance.ID, api.IssuanceWaiting)
	e.call("POST", fmt.Sprintf("/issuances/%d/continue", waiting.ID), map[string]bool{"force": true}, 202, nil)
	failed := e.waitIssuance(waiting.ID, api.IssuanceFailed)
	saved, _ := e.db.GetIssuance(context.Background(), failed.ID)
	if failed.Attempts != 1 || saved.KeyPEM != "" {
		t.Fatalf("permanent failure retained retry state: %+v", failed)
	}
}
