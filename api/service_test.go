package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/acme-go/api"
	"github.com/lsongdev/acme-go/store"
)

type env struct {
	t       *testing.T
	ca      *fakeCA
	db      *store.SQLite
	service *api.Service
	server  *httptest.Server

	mu  sync.Mutex
	dns map[string][]string // published TXT records
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	e := &env{t: t, ca: newFakeCA(t), dns: map[string][]string{}}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "acme.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e.db = db
	e.newService()

	e.ca.fetchHTTP01 = func(domain, token string) (string, bool) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "http://"+domain+"/.well-known/acme-challenge/"+token, nil)
		e.service.HTTP01Handler().ServeHTTP(recorder, request)
		return recorder.Body.String(), recorder.Code == 200
	}
	e.ca.lookupTXT = e.lookup
	t.Cleanup(api.SetLookupTXT(func(_ context.Context, _, name string) ([]string, error) {
		return e.lookup(name), nil
	}))
	return e
}

// newService starts a service on the env's database, as after a restart.
func (e *env) newService() {
	service, err := api.NewService(context.Background(), e.db, api.Options{ValidationTimeout: 10 * time.Second, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { service.Close() })
	e.service = service
	e.server = httptest.NewServer(service.Handler())
	e.t.Cleanup(e.server.Close)
}

func (e *env) lookup(name string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dns[name]
}

func (e *env) publish(name, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dns[name] = append(e.dns[name], value)
}

// call performs an API request, checks the status and decodes "data" into
// out when given.
func (e *env) call(method, path string, body any, wantStatus int, out any) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequest(method, e.server.URL+path, reader)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		e.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != wantStatus {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.StatusCode, wantStatus, data)
	}
	if out != nil {
		var envelope struct{ Data json.RawMessage }
		if err := json.Unmarshal(data, &envelope); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}
}

func (e *env) registerAccount() api.Account {
	e.t.Helper()
	var provider api.Provider
	e.call("POST", "/providers", api.CreateProviderRequest{Name: "Fake CA", DirectoryURL: e.ca.DirectoryURL()}, 201, &provider)
	var directory api.ProviderDirectory
	e.call("GET", fmt.Sprintf("/providers/%d/directory", provider.ID), nil, 200, &directory)
	if directory.TermsOfService == "" {
		e.t.Fatal("directory has no terms of service")
	}
	e.call("POST", "/accounts", api.RegisterAccountRequest{ProviderID: provider.ID, Email: "admin@example.com"}, 422, nil)
	var account api.Account
	e.call("POST", "/accounts", api.RegisterAccountRequest{ProviderID: provider.ID, Email: "admin@example.com", AgreeTerms: true}, 201, &account)
	if account.Status != "valid" || account.URL == "" || account.Thumbprint == "" {
		e.t.Fatalf("account = %+v", account)
	}
	return account
}

// waitIssuance polls until the issuance reaches one of the statuses.
func (e *env) waitIssuance(id int64, statuses ...string) api.Issuance {
	e.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var issuance api.Issuance
		e.call("GET", fmt.Sprintf("/issuances/%d", id), nil, 200, &issuance)
		if slices.Contains(statuses, issuance.Status) {
			return issuance
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("issuance %d is %s/%s (error %q), want %v", id, issuance.Status, issuance.Step, issuance.Error, statuses)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type created struct {
	Certificate api.Certificate `json:"certificate"`
	Issuance    api.Issuance    `json:"issuance"`
}

func TestHTTP01IssueRenewRevoke(t *testing.T) {
	e := newEnv(t)
	account := e.registerAccount()

	var result created
	e.call("POST", "/certificates", api.CreateCertificateRequest{
		Domains: []string{"Example.com", "www.example.com", "example.com"}, AccountID: account.ID, ChallengeType: "http-01", AutoRenew: true,
	}, 202, &result)
	if got := result.Certificate.Domains; !slices.Equal(got, []string{"example.com", "www.example.com"}) {
		t.Fatalf("domains = %v", got)
	}
	issuance := e.waitIssuance(result.Issuance.ID, api.IssuanceSucceeded, api.IssuanceFailed)
	if issuance.Status != api.IssuanceSucceeded {
		t.Fatalf("issuance failed at %s: %s", issuance.Step, issuance.Error)
	}

	var certificate api.Certificate
	e.call("GET", fmt.Sprintf("/certificates/%d", result.Certificate.ID), nil, 200, &certificate)
	if certificate.Status != api.CertificateActive || certificate.Issuer != "Fake ACME Root" || certificate.NotAfter.IsZero() {
		t.Fatalf("certificate = %+v", certificate)
	}
	if !strings.Contains(certificate.CertPEM, "BEGIN CERTIFICATE") {
		t.Fatal("certificate has no PEM")
	}
	firstKey := e.downloadKey(certificate.ID)

	var orders []api.Order
	e.call("GET", "/orders", nil, 200, &orders)
	if len(orders) != 1 || orders[0].Status != "valid" || orders[0].IssuanceID != issuance.ID {
		t.Fatalf("orders = %+v", orders)
	}

	// Renewal issues a new certificate with a new key.
	var renewal api.Issuance
	e.call("POST", fmt.Sprintf("/certificates/%d/renew", certificate.ID), nil, 202, &renewal)
	if renewal = e.waitIssuance(renewal.ID, api.IssuanceSucceeded, api.IssuanceFailed); renewal.Status != api.IssuanceSucceeded {
		t.Fatalf("renewal failed: %s", renewal.Error)
	}
	var renewed api.Certificate
	e.call("GET", fmt.Sprintf("/certificates/%d", certificate.ID), nil, 200, &renewed)
	if renewed.SerialNumber == certificate.SerialNumber {
		t.Fatal("renewal did not replace the certificate")
	}
	if e.downloadKey(certificate.ID) == firstKey {
		t.Fatal("renewal reused the private key")
	}

	// Accounts in use cannot be deleted; providers with accounts neither.
	e.call("DELETE", fmt.Sprintf("/accounts/%d", account.ID), nil, 409, nil)
	e.call("DELETE", fmt.Sprintf("/providers/%d", account.ProviderID), nil, 409, nil)

	var revoked api.Certificate
	e.call("POST", fmt.Sprintf("/certificates/%d/revoke", certificate.ID), map[string]int{"reason": 4}, 200, &revoked)
	if revoked.Status != api.CertificateRevoked || revoked.AutoRenew || len(e.ca.revoked) != 1 {
		t.Fatalf("revoked = %+v, CA revocations = %d", revoked, len(e.ca.revoked))
	}
	e.call("POST", fmt.Sprintf("/certificates/%d/renew", certificate.ID), nil, 422, nil)

	e.call("DELETE", fmt.Sprintf("/certificates/%d", certificate.ID), nil, 204, nil)
	e.call("GET", fmt.Sprintf("/certificates/%d", certificate.ID), nil, 404, nil)
	e.call("DELETE", fmt.Sprintf("/accounts/%d", account.ID), nil, 204, nil)
}

// downloadKey fetches the zip download and returns the private key in it.
func (e *env) downloadKey(id int64) string {
	e.t.Helper()
	response, err := http.Get(fmt.Sprintf("%s/certificates/%d/download", e.server.URL, id))
	if err != nil {
		e.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		e.t.Fatalf("download: %v: %s", err, data)
	}
	files := map[string]string{}
	for _, file := range archive.File {
		f, _ := file.Open()
		content, _ := io.ReadAll(f)
		f.Close()
		files[file.Name] = string(content)
	}
	key := files["example.com/privkey.pem"]
	if !strings.Contains(key, "PRIVATE KEY") || !strings.Contains(files["example.com/fullchain.pem"], "CERTIFICATE") {
		e.t.Fatalf("archive files = %v", archive.File)
	}
	return key
}

func TestManualDNS01Wildcard(t *testing.T) {
	e := newEnv(t)
	account := e.registerAccount()

	e.call("POST", "/certificates", api.CreateCertificateRequest{
		Domains: []string{"*.example.org"}, AccountID: account.ID, ChallengeType: "http-01",
	}, 422, nil)

	var result created
	e.call("POST", "/certificates", api.CreateCertificateRequest{
		Domains: []string{"*.example.org", "example.org"}, AccountID: account.ID, ChallengeType: "dns-01",
	}, 202, &result)
	issuance := e.waitIssuance(result.Issuance.ID, api.IssuanceWaiting)
	if len(issuance.DNSRecords) != 2 {
		t.Fatalf("records = %+v", issuance.DNSRecords)
	}
	for _, record := range issuance.DNSRecords {
		if record.Name != "_acme-challenge.example.org" {
			t.Fatalf("record name = %q", record.Name)
		}
	}

	// Continuing before the records are visible keeps waiting.
	e.call("POST", fmt.Sprintf("/issuances/%d/continue", issuance.ID), nil, 202, nil)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(issuance.Error, "not visible") {
		if time.Now().After(deadline) {
			t.Fatalf("issuance = %+v", issuance)
		}
		time.Sleep(20 * time.Millisecond)
		e.call("GET", fmt.Sprintf("/issuances/%d", issuance.ID), nil, 200, &issuance)
	}
	if issuance.Status != api.IssuanceWaiting {
		t.Fatalf("status = %s", issuance.Status)
	}

	for _, record := range issuance.DNSRecords {
		e.publish(record.Name, record.Value)
	}
	e.call("POST", fmt.Sprintf("/issuances/%d/continue", issuance.ID), map[string]bool{"force": false}, 202, nil)
	if issuance = e.waitIssuance(issuance.ID, api.IssuanceSucceeded, api.IssuanceFailed); issuance.Status != api.IssuanceSucceeded {
		t.Fatalf("issuance failed at %s: %s", issuance.Step, issuance.Error)
	}
	var certificate api.Certificate
	e.call("GET", fmt.Sprintf("/certificates/%d", result.Certificate.ID), nil, 200, &certificate)
	if certificate.Status != api.CertificateActive {
		t.Fatalf("certificate = %+v", certificate)
	}
}

func TestCancelAndRestart(t *testing.T) {
	e := newEnv(t)
	account := e.registerAccount()

	var result created
	e.call("POST", "/certificates", api.CreateCertificateRequest{Domains: []string{"a.example.net"}, AccountID: account.ID, ChallengeType: "dns-01"}, 202, &result)
	e.waitIssuance(result.Issuance.ID, api.IssuanceWaiting)
	// Only one issuance per certificate at a time.
	e.call("POST", fmt.Sprintf("/certificates/%d/renew", result.Certificate.ID), nil, 409, nil)
	e.call("DELETE", fmt.Sprintf("/certificates/%d", result.Certificate.ID), nil, 409, nil)

	e.call("POST", fmt.Sprintf("/issuances/%d/cancel", result.Issuance.ID), nil, 202, nil)
	issuance := e.waitIssuance(result.Issuance.ID, api.IssuanceFailed)
	if issuance.Error != "canceled" {
		t.Fatalf("error = %q", issuance.Error)
	}
	var certificate api.Certificate
	e.call("GET", fmt.Sprintf("/certificates/%d", result.Certificate.ID), nil, 200, &certificate)
	if certificate.Status != api.CertificateFailed {
		t.Fatalf("certificate status = %s", certificate.Status)
	}

	// An issuance left waiting when the process stops is failed on restart.
	var retry api.Issuance
	e.call("POST", fmt.Sprintf("/certificates/%d/renew", result.Certificate.ID), nil, 202, &retry)
	e.waitIssuance(retry.ID, api.IssuanceWaiting)
	e.service.Close()
	e.newService()
	issuance = e.waitIssuance(retry.ID, api.IssuanceFailed)
	if !strings.HasPrefix(issuance.Error, "interrupted") {
		t.Fatalf("error = %q", issuance.Error)
	}
}

func TestAPIErrors(t *testing.T) {
	e := newEnv(t)
	e.call("GET", "/certificates/99", nil, 404, nil)
	e.call("GET", "/certificates/abc", nil, 400, nil)
	e.call("GET", "/nope", nil, 404, nil)
	e.call("POST", "/providers", map[string]string{"name": "x", "directory_url": "ftp://x"}, 422, nil)
	e.call("POST", "/providers", map[string]string{"unknown": "field"}, 400, nil)
	e.call("POST", "/providers", api.CreateProviderRequest{Name: "Let's Encrypt", DirectoryURL: "https://example.com/dir"}, 409, nil)
	e.call("POST", "/certificates", api.CreateCertificateRequest{Domains: []string{"example.com"}, AccountID: 42, ChallengeType: "http-01"}, 422, nil)

	var providers []api.Provider
	e.call("GET", "/providers", nil, 200, &providers)
	if len(providers) != len(store.DefaultProviders) {
		t.Fatalf("providers = %+v", providers)
	}
	var certificates []api.Certificate
	e.call("GET", "/certificates", nil, 200, &certificates)
	if certificates == nil || len(certificates) != 0 {
		t.Fatalf("certificates = %#v, want empty list", certificates)
	}
}

func TestNormalizeDomains(t *testing.T) {
	got, err := api.NormalizeDomains([]string{" WWW.Example.com. ", "*.example.com", "", "www.example.com"})
	if err != nil || !slices.Equal(got, []string{"www.example.com", "*.example.com"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"localhost", "exa mple.com", "-a.example.com", "a..example.com", "*.*.example.com", "http://example.com"} {
		if _, err := api.NormalizeDomains([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
