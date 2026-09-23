package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func decodeJWS(t *testing.T, r *http.Request) (Header, []byte) {
	t.Helper()
	var jws jsonWebSignature
	if err := json.NewDecoder(r.Body).Decode(&jws); err != nil {
		t.Fatal(err)
	}
	protected, err := base64.RawURLEncoding.DecodeString(jws.Protected)
	if err != nil {
		t.Fatal(err)
	}
	var header Header
	if err := json.Unmarshal(protected, &header); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(jws.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return header, payload
}

func TestNewClientDoesNotFetchDirectory(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := NewClient(&Config{DirectoryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if client.Directory != nil {
		t.Fatal("directory should be lazy")
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestGetOrderUsesPOSTAsGET(t *testing.T) {
	ctx := context.Background()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/directory":
			json.NewEncoder(w).Encode(map[string]string{
				"newNonce": server.URL + "/nonce",
				"newOrder": server.URL + "/order",
			})
		case "/nonce":
			if r.Method != http.MethodHead {
				t.Fatalf("nonce method = %s, want HEAD", r.Method)
			}
			w.Header().Set("Replay-Nonce", "nonce-1")
		case "/order":
			if r.Method != http.MethodPost {
				t.Fatalf("order method = %s, want POST", r.Method)
			}
			header, payload := decodeJWS(t, r)
			if header.KID != "https://ca.example/acct/1" {
				t.Fatalf("kid = %q", header.KID)
			}
			if header.Nonce != "nonce-1" {
				t.Fatalf("nonce = %q", header.Nonce)
			}
			if len(payload) != 0 {
				t.Fatalf("POST-as-GET payload = %q, want empty", payload)
			}
			w.Header().Set("Replay-Nonce", "nonce-2")
			json.NewEncoder(w).Encode(&OrderResponse{Status: OrderStatusPending})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(&Config{
		DirectoryURL: server.URL + "/directory",
		AccountURL:   "https://ca.example/acct/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	client.PrivateKey = testKey(t)

	order, err := client.GetOrder(ctx, server.URL+"/order")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusPending {
		t.Fatalf("status = %q", order.Status)
	}
}

func TestCompleteChallengeSendsEmptyObject(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, payload := decodeJWS(t, r)
		if string(payload) != "{}" {
			t.Fatalf("challenge payload = %q, want {}", payload)
		}
		w.Header().Set("Replay-Nonce", "nonce-2")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		PrivateKey: testKey(t),
		AccountURL: "https://ca.example/acct/1",
		nonce:      "nonce-1",
	}
	if err := client.CompleteChallenge(ctx, server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterUsesJWKAndStoresAccountURL(t *testing.T) {
	ctx := context.Background()
	const accountURL = "https://ca.example/acct/2"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header, _ := decodeJWS(t, r)
		if header.KID != "" {
			t.Fatalf("newAccount included kid %q", header.KID)
		}
		if len(header.JWK) == 0 {
			t.Fatal("newAccount did not include jwk")
		}
		w.Header().Set("Location", accountURL)
		w.Header().Set("Replay-Nonce", "nonce-2")
		json.NewEncoder(w).Encode(&AccountResponse{Status: "valid"})
	}))
	defer server.Close()

	config := &Config{}
	client := &Client{
		Config:     config,
		Directory:  &Directory{NewAccount: server.URL},
		PrivateKey: testKey(t),
		AccountURL: "https://ca.example/acct/old",
		nonce:      "nonce-1",
	}
	url, _, err := client.Register(ctx, &AccountRequest{TermsOfServiceAgreed: true})
	if err != nil {
		t.Fatal(err)
	}
	if url != accountURL || client.AccountURL != accountURL || config.AccountURL != accountURL {
		t.Fatalf("account URL = %q, client = %q, config = %q", url, client.AccountURL, config.AccountURL)
	}
}

func TestBadNonceRetriesWithResponseNonce(t *testing.T) {
	ctx := context.Background()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requests.Add(1)
		header, payload := decodeJWS(t, r)
		if len(payload) != 0 {
			t.Fatalf("payload = %q, want empty", payload)
		}
		if request == 1 {
			if header.Nonce != "nonce-1" {
				t.Fatalf("first nonce = %q", header.Nonce)
			}
			w.Header().Set("Replay-Nonce", "nonce-2")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(&Response{Type: badNonceError, Detail: "bad nonce"})
			return
		}
		if header.Nonce != "nonce-2" {
			t.Fatalf("retry nonce = %q", header.Nonce)
		}
		w.Header().Set("Replay-Nonce", "nonce-3")
		json.NewEncoder(w).Encode(&OrderResponse{Status: OrderStatusPending})
	}))
	defer server.Close()

	client := &Client{
		PrivateKey: testKey(t),
		AccountURL: "https://ca.example/acct/1",
		nonce:      "nonce-1",
	}
	if _, err := client.GetOrder(ctx, server.URL); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestSigningErrorDoesNotSendRequest(t *testing.T) {
	ctx := context.Background()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client := &Client{nonce: "nonce-1"}
	if _, _, err := client.post(ctx, server.URL, struct{}{}); err == nil {
		t.Fatal("expected signing error")
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestDeactivateAccountReturnsHTTPError(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "nonce-2")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"type":   "urn:ietf:params:acme:error:serverInternal",
			"detail": "boom",
		})
	}))
	defer server.Close()

	client := &Client{
		PrivateKey: testKey(t),
		AccountURL: "https://ca.example/acct/1",
		nonce:      "nonce-1",
	}
	err := client.DeactivateAccount(ctx, server.URL)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v", err)
	}
}

func TestImportKeyRejectsInvalidPEM(t *testing.T) {
	client := &Client{}
	if err := client.ImportKey("not a PEM key"); err == nil {
		t.Fatal("expected invalid PEM error")
	}
}

func TestLegacyECKeyImport(t *testing.T) {
	key := testKey(t)
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	client := &Client{}
	if err := client.ImportKey(string(data)); err != nil {
		t.Fatal(err)
	}
	if _, ok := client.PrivateKey.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("key type = %T", client.PrivateKey)
	}
}

func TestLegacyRSAKeyImport(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	client := &Client{}
	if err := client.ImportKey(string(data)); err != nil {
		t.Fatal(err)
	}
	if _, ok := client.PrivateKey.(*rsa.PrivateKey); !ok {
		t.Fatalf("key type = %T", client.PrivateKey)
	}
}

func TestRSAKeyPKCS8RoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{PrivateKey: key}

	data, err := client.ExportKey()
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(data))
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("PEM type = %v, want PRIVATE KEY", block)
	}

	restored := &Client{}
	if err := restored.ImportKey(data); err != nil {
		t.Fatal(err)
	}
	if _, ok := restored.PrivateKey.(*rsa.PrivateKey); !ok {
		t.Fatalf("key type = %T", restored.PrivateKey)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCustomHTTPClient(t *testing.T) {
	ctx := context.Background()
	var called atomic.Bool
	client := &Client{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				called.Store(true)
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       http.NoBody,
					Request:    req,
				}, nil
			}),
		},
	}

	res, err := client.request(ctx, http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !called.Load() {
		t.Fatal("custom HTTP client was not used")
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &Client{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			}),
		},
	}
	if _, err := client.request(ctx, http.MethodGet, "https://example.test", nil); err == nil {
		t.Fatal("expected context cancellation")
	}
}
