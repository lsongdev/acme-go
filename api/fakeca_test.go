package api_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCA is a minimal in-memory ACME server. It does not verify JWS
// signatures; it validates challenges through the hooks below.
type fakeCA struct {
	t      *testing.T
	server *httptest.Server
	key    *ecdsa.PrivateKey
	cert   *x509.Certificate

	// fetchHTTP01 returns the body served for an HTTP-01 token.
	fetchHTTP01 func(domain, token string) (string, bool)
	// lookupTXT returns the TXT values published at name.
	lookupTXT func(name string) []string

	mu         sync.Mutex
	nextID     int
	orders     map[string]*fakeOrder
	authzs     map[string]*fakeAuthz
	challenges map[string]*fakeAuthz // challenge ID -> owning authorization
	certs      map[string]string
	revoked    []string
	accounts   map[string]string // account URL -> status
}

type fakeOrder struct {
	Status         string            `json:"status"`
	Identifiers    []json.RawMessage `json:"identifiers"`
	Authorizations []string          `json:"authorizations"`
	Finalize       string            `json:"finalize"`
	Certificate    string            `json:"certificate,omitempty"`
	domains        []string
}

type fakeAuthz struct {
	Status     string          `json:"status"`
	Identifier map[string]any  `json:"identifier"`
	Wildcard   bool            `json:"wildcard,omitempty"`
	Challenges []fakeChallenge `json:"challenges"`
}

type fakeChallenge struct {
	Type   string         `json:"type"`
	URL    string         `json:"url"`
	Token  string         `json:"token"`
	Status string         `json:"status"`
	Error  map[string]any `json:"error,omitempty"`
}

func newFakeCA(t *testing.T) *fakeCA {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Fake ACME Root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	ca := &fakeCA{
		t: t, key: key, cert: cert,
		orders:     map[string]*fakeOrder{},
		authzs:     map[string]*fakeAuthz{},
		challenges: map[string]*fakeAuthz{},
		certs:      map[string]string{},
		accounts:   map[string]string{},
	}
	ca.server = httptest.NewServer(http.HandlerFunc(ca.serve))
	t.Cleanup(ca.server.Close)
	return ca
}

func (ca *fakeCA) DirectoryURL() string { return ca.server.URL + "/directory" }

func (ca *fakeCA) id() string {
	ca.nextID++
	return fmt.Sprint(ca.nextID)
}

func (ca *fakeCA) url(path string) string { return ca.server.URL + path }

// payload decodes the JWS payload of a request; empty for POST-as-GET.
func payload(r *http.Request) []byte {
	var jws struct {
		Payload string `json:"payload"`
	}
	_ = json.NewDecoder(r.Body).Decode(&jws)
	data, _ := base64.RawURLEncoding.DecodeString(jws.Payload)
	return data
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (ca *fakeCA) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", time.Now().UnixNano()))
	path := r.URL.Path
	if path == "/directory" {
		writeJSON(w, 200, map[string]any{
			"newNonce": ca.url("/nonce"), "newAccount": ca.url("/new-account"),
			"newOrder": ca.url("/new-order"), "revokeCert": ca.url("/revoke"),
			"meta": map[string]any{"termsOfService": ca.url("/terms")},
		})
		return
	}
	if path == "/nonce" {
		w.WriteHeader(http.StatusOK)
		return
	}
	body := payload(r)
	ca.mu.Lock()
	defer ca.mu.Unlock()
	switch {
	case path == "/new-account":
		var request struct {
			TermsOfServiceAgreed bool `json:"termsOfServiceAgreed"`
		}
		_ = json.Unmarshal(body, &request)
		if !request.TermsOfServiceAgreed {
			writeJSON(w, 403, map[string]string{"type": "urn:ietf:params:acme:error:userActionRequired", "detail": "agree to the terms"})
			return
		}
		accountURL := ca.url("/account/" + ca.id())
		ca.accounts[accountURL] = "valid"
		w.Header().Set("Location", accountURL)
		writeJSON(w, 201, map[string]any{"status": "valid"})
	case strings.HasPrefix(path, "/account/"):
		var request struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(body, &request)
		if request.Status != "" {
			ca.accounts[ca.url(path)] = request.Status
		}
		writeJSON(w, 200, map[string]any{"status": ca.accounts[ca.url(path)]})
	case path == "/new-order":
		var request struct {
			Identifiers []struct {
				Value string `json:"value"`
			} `json:"identifiers"`
		}
		_ = json.Unmarshal(body, &request)
		orderID := ca.id()
		order := &fakeOrder{Status: "pending", Finalize: ca.url("/finalize/" + orderID)}
		for _, identifier := range request.Identifiers {
			order.domains = append(order.domains, identifier.Value)
			raw, _ := json.Marshal(map[string]string{"type": "dns", "value": identifier.Value})
			order.Identifiers = append(order.Identifiers, raw)
			authzID := ca.id()
			base, wildcard := strings.CutPrefix(identifier.Value, "*.")
			authz := &fakeAuthz{Status: "pending", Identifier: map[string]any{"type": "dns", "value": base}, Wildcard: wildcard}
			types := []string{"http-01", "dns-01"}
			if wildcard {
				types = []string{"dns-01"}
			}
			for _, kind := range types {
				challengeID := ca.id()
				authz.Challenges = append(authz.Challenges, fakeChallenge{Type: kind, URL: ca.url("/challenge/" + challengeID), Token: "token" + challengeID, Status: "pending"})
				ca.challenges[challengeID] = authz
			}
			ca.authzs[authzID] = authz
			order.Authorizations = append(order.Authorizations, ca.url("/authz/"+authzID))
		}
		ca.orders[orderID] = order
		w.Header().Set("Location", ca.url("/order/"+orderID))
		writeJSON(w, 201, order)
	case strings.HasPrefix(path, "/order/"):
		order := ca.orders[strings.TrimPrefix(path, "/order/")]
		ca.refresh(order)
		writeJSON(w, 200, order)
	case strings.HasPrefix(path, "/authz/"):
		writeJSON(w, 200, ca.authzs[strings.TrimPrefix(path, "/authz/")])
	case strings.HasPrefix(path, "/challenge/"):
		id := strings.TrimPrefix(path, "/challenge/")
		authz := ca.challenges[id]
		for i := range authz.Challenges {
			challenge := &authz.Challenges[i]
			if challenge.URL != ca.url(path) {
				continue
			}
			ca.validate(authz, challenge)
			writeJSON(w, 200, challenge)
			return
		}
	case strings.HasPrefix(path, "/finalize/"):
		id := strings.TrimPrefix(path, "/finalize/")
		order := ca.orders[id]
		ca.refresh(order)
		if order.Status != "ready" {
			writeJSON(w, 403, map[string]string{"type": "urn:ietf:params:acme:error:orderNotReady"})
			return
		}
		var request struct {
			CSR string `json:"csr"`
		}
		_ = json.Unmarshal(body, &request)
		der, _ := base64.RawURLEncoding.DecodeString(request.CSR)
		csr, err := x509.ParseCertificateRequest(der)
		if err != nil {
			writeJSON(w, 400, map[string]string{"type": "urn:ietf:params:acme:error:badCSR", "detail": err.Error()})
			return
		}
		ca.certs[id] = ca.issue(csr)
		order.Status = "valid"
		order.Certificate = ca.url("/cert/" + id)
		writeJSON(w, 200, order)
	case strings.HasPrefix(path, "/cert/"):
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		_, _ = w.Write([]byte(ca.certs[strings.TrimPrefix(path, "/cert/")]))
	case path == "/revoke":
		var request struct {
			Certificate string `json:"certificate"`
		}
		_ = json.Unmarshal(body, &request)
		ca.revoked = append(ca.revoked, request.Certificate)
		w.WriteHeader(200)
	default:
		http.NotFound(w, r)
	}
}

// validate checks a challenge synchronously, as soon as it is triggered.
func (ca *fakeCA) validate(authz *fakeAuthz, challenge *fakeChallenge) {
	domain := authz.Identifier["value"].(string)
	ok := false
	switch challenge.Type {
	case "http-01":
		body, found := ca.fetchHTTP01(domain, challenge.Token)
		ok = found && strings.HasPrefix(body, challenge.Token+".")
	case "dns-01":
		// The expected digest depends on the account thumbprint, which the
		// fake does not track; accept any non-empty base64url SHA-256 value.
		for _, value := range ca.lookupTXT("_acme-challenge." + domain) {
			if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil && len(decoded) == sha256.Size {
				ok = true
			}
		}
	}
	if ok {
		challenge.Status, authz.Status = "valid", "valid"
	} else {
		challenge.Status, authz.Status = "invalid", "invalid"
		challenge.Error = map[string]any{"type": "urn:ietf:params:acme:error:unauthorized", "detail": "challenge failed for " + domain}
	}
}

func (ca *fakeCA) refresh(order *fakeOrder) {
	if order.Status != "pending" {
		return
	}
	ready := true
	for _, authzURL := range order.Authorizations {
		authz := ca.authzs[authzURL[strings.LastIndex(authzURL, "/")+1:]]
		switch authz.Status {
		case "invalid":
			order.Status = "invalid"
			return
		case "pending":
			ready = false
		}
	}
	if ready {
		order.Status = "ready"
	}
}

func (ca *fakeCA) issue(csr *x509.CertificateRequest) string {
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: csr.Subject.CommonName},
		DNSNames:     csr.DNSNames,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		ca.t.Errorf("issue certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) +
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}))
}
