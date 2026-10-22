package acme

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// pollServer serves a directory and a resource whose successive bodies are
// given by responses; the last one repeats.
func pollServer(t *testing.T, responses ...string) (*Client, string, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Replay-Nonce", "nonce")
		switch r.URL.Path {
		case "/directory":
			w.Write([]byte(`{"newNonce":"` + server.URL + `/nonce"}`))
		case "/nonce":
		case "/resource":
			n := int(calls.Add(1)) - 1
			w.Header().Set("Retry-After", "0")
			w.Write([]byte(responses[min(n, len(responses)-1)]))
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(&Config{DirectoryURL: server.URL + "/directory", AccountURL: server.URL + "/account"})
	if err != nil {
		t.Fatal(err)
	}
	client.PrivateKey = testKey(t)
	return client, server.URL + "/resource", &calls
}

func TestWaitOrder(t *testing.T) {
	client, url, calls := pollServer(t, `{"status":"pending"}`, `{"status":"processing"}`, `{"status":"valid","certificate":"c"}`)
	order, err := client.WaitOrder(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusValid || order.Certificate != "c" || calls.Load() != 3 {
		t.Fatalf("order = %#v after %d calls", order, calls.Load())
	}

	client, url, _ = pollServer(t, `{"status":"invalid"}`)
	if _, err := client.WaitOrder(context.Background(), url); err == nil {
		t.Fatal("invalid order returned no error")
	}
}

func TestWaitAuthorizationReturnsChallengeProblem(t *testing.T) {
	client, url, _ := pollServer(t,
		`{"status":"pending"}`,
		`{"status":"invalid","challenges":[{"type":"http-01","status":"invalid","error":{"type":"urn:ietf:params:acme:error:connection","detail":"refused"}}]}`,
	)
	_, err := client.WaitAuthorization(context.Background(), url)
	var problem *Response
	if !errors.As(err, &problem) || !strings.HasSuffix(problem.Type, ":connection") {
		t.Fatalf("error = %v", err)
	}
}

func TestWaitHonorsContext(t *testing.T) {
	client, url, _ := pollServer(t, `{"status":"pending"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := client.WaitOrder(ctx, url); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestRetryAfter(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"":      time.Second,
		"3":     3 * time.Second,
		"3600":  maxPollInterval,
		"bogus": time.Second,
	} {
		headers := http.Header{}
		headers.Set("Retry-After", value)
		if got := retryAfter(headers, time.Second); got != want {
			t.Errorf("Retry-After %q = %v, want %v", value, got, want)
		}
	}
}

func TestClientIsSafeForConcurrentUse(t *testing.T) {
	client, url, _ := pollServer(t, `{"status":"valid"}`)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.WaitOrder(context.Background(), url); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
