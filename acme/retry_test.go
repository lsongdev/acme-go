package acme

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastRetry() RetryPolicy {
	return RetryPolicy{MaxAttempts: 4, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}
}

func TestReadRetriesWithFreshJWS(t *testing.T) {
	var calls atomic.Int32
	var nonceCalls atomic.Int32
	var nonces []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			nonceCalls.Add(1)
			w.Header().Set("Replay-Nonce", "fresh-nonce")
			return
		}
		header, _ := decodeJWS(t, r)
		nonces = append(nonces, header.Nonce)
		if calls.Add(1) == 1 {
			// A proxy may return a non-JSON error without Replay-Nonce.
			w.WriteHeader(503)
			w.Write([]byte("temporarily unavailable"))
			return
		}
		w.Write([]byte(`{"status":"valid"}`))
	}))
	defer server.Close()
	client := &Client{PrivateKey: testKey(t), AccountURL: "account", Directory: &Directory{NewNonce: server.URL}, nonce: "first-nonce", RetryPolicy: fastRetry()}
	if _, err := client.GetOrder(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || nonceCalls.Load() != 1 || strings.Join(nonces, ",") != "first-nonce,fresh-nonce" {
		t.Fatalf("requests=%d nonce requests=%d nonces=%v", calls.Load(), nonceCalls.Load(), nonces)
	}
}

func TestReadRetryLimitsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status, attempts int
		retryAfter       string
		timeout          time.Duration
	}{
		{"temporary", 503, 4, "", time.Second},
		{"permanent", 403, 1, "", time.Second},
		{"rate limit respects server delay", 429, 1, "2", 20 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", tc.retryAfter)
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			client := &Client{RetryPolicy: fastRetry(), Config: &Config{DirectoryURL: server.URL}}
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			_, err := client.GetDirectory(ctx)
			if err == nil || int(calls.Load()) != tc.attempts {
				t.Fatalf("error=%v requests=%d", err, calls.Load())
			}
			if tc.status == 429 && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMutatingPostIsNotReplayedOnTransportFailure(t *testing.T) {
	var calls atomic.Int32
	client := &Client{PrivateKey: testKey(t), nonce: "nonce", RetryPolicy: fastRetry(), HTTPClient: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, io.EOF
	})}}
	_, _, err := client.post(context.Background(), "https://ca.example/new-order", struct{}{})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("error=%v requests=%d", err, calls.Load())
	}
}

func TestFinalizeReconcilesLostResponse(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted", false: "not accepted"}[accepted], func(t *testing.T) {
			var posts int
			state := "ready"
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Replay-Nonce", "nonce")
				_, body := decodeJWS(t, r)
				if r.URL.Path == "/finalize" {
					posts++
					var payload map[string]string
					json.Unmarshal(body, &payload)
					if payload["csr"] != "Y3Ny" {
						t.Errorf("CSR changed: %s", body)
					}
					if posts == 1 {
						if accepted {
							state = "valid"
						}
						w.WriteHeader(503)
						return
					}
					state = "valid"
				}
				json.NewEncoder(w).Encode(OrderResponse{Status: state, Finalize: server.URL + "/finalize", Certificate: "certificate"})
			}))
			defer server.Close()
			client := &Client{PrivateKey: testKey(t), nonce: "nonce", RetryPolicy: fastRetry()}
			order, err := client.FinalizeAndWait(context.Background(), server.URL+"/order", []byte("csr"))
			want := 2
			if accepted {
				want = 1
			}
			if err != nil || order.Status != "valid" || posts != want {
				t.Fatalf("order=%+v error=%v finalize requests=%d", order, err, posts)
			}
		})
	}
}

func TestDefaultHTTPTimeouts(t *testing.T) {
	client := NewHTTPClient(HTTPOptions{})
	defer client.CloseIdleConnections()
	if client.Timeout != time.Minute || client.Transport.(*http.Transport).TLSHandshakeTimeout != 30*time.Second {
		t.Fatal("unexpected defaults")
	}
	if http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout != 10*time.Second {
		t.Fatal("changed process-wide transport")
	}
}
