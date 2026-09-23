package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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

func TestGetOrderUsesPOSTAsGET(t *testing.T) {
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

	order, err := client.GetOrder(server.URL + "/order")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusPending {
		t.Fatalf("status = %q", order.Status)
	}
}

func TestCompleteChallengeSendsEmptyObject(t *testing.T) {
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
	if err := client.CompleteChallenge(server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterUsesJWKAndStoresAccountURL(t *testing.T) {
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

	client := &Client{
		Directory:  &Directory{NewAccount: server.URL},
		PrivateKey: testKey(t),
		AccountURL: "https://ca.example/acct/old",
		nonce:      "nonce-1",
	}
	url, _, err := client.Register(&AccountRequest{TermsOfServiceAgreed: true})
	if err != nil {
		t.Fatal(err)
	}
	if url != accountURL || client.AccountURL != accountURL {
		t.Fatalf("account URL = %q, client = %q", url, client.AccountURL)
	}
}

func TestBadNonceRetriesWithResponseNonce(t *testing.T) {
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
	if _, err := client.GetOrder(server.URL); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestSigningErrorDoesNotSendRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client := &Client{nonce: "nonce-1"}
	if _, _, err := client.post(server.URL, struct{}{}); err == nil {
		t.Fatal("expected signing error")
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestDeactivateAccountReturnsHTTPError(t *testing.T) {
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
	err := client.DeactivateAccount(server.URL)
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
