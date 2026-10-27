package acme

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

// RetryPolicy bounds attempts, including the first request. Zero values mean
// four attempts, a one-second initial delay and a thirty-second maximum delay.
// MaxAttempts=1 disables retries. Delays include jitter.
type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func (p RetryPolicy) Attempts() int {
	if p.MaxAttempts > 0 {
		return p.MaxAttempts
	}
	return 4
}

// Delay returns the backoff after the given failed attempt (starting at 1).
func (p RetryPolicy) Delay(attempt int) time.Duration {
	initial, limit := p.InitialDelay, p.MaxDelay
	if initial <= 0 {
		initial = time.Second
	}
	if limit <= 0 {
		limit = 30 * time.Second
	}
	delay := min(initial, limit)
	for n := 1; n < attempt && delay < limit; n++ {
		if delay > limit/2 {
			delay = limit
			break
		}
		delay *= 2
	}
	// Equal jitter avoids synchronized retries without a zero-delay loop.
	return delay/2 + time.Duration(rand.Int64N(int64(delay-delay/2)+1))
}

// IsRetryable identifies transient transport errors and HTTP responses. A
// caller must also check its context: an expired parent budget is never retried.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var problem *Response
	if errors.As(err, &problem) {
		switch problem.Status {
		case 408, 429, 500, 502, 503, 504:
			return true
		}
		return false
	}
	var certError *tls.CertificateVerificationError
	if errors.As(err, &certError) {
		return false
	}
	var network net.Error
	return errors.As(err, &network) && (network.Timeout() || network.Temporary()) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE)
}

// RetryTime is the server's earliest retry time, or zero if none was supplied.
func RetryTime(err error) time.Time {
	var problem *Response
	if errors.As(err, &problem) {
		return problem.RetryAt
	}
	return time.Time{}
}

func parseRetryTime(value string) time.Time {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		return time.Now().Add(time.Duration(seconds) * time.Second)
	}
	date, _ := http.ParseTime(value)
	return date
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(max(delay, 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

// retryRead is used only for reads (including ACME POST-as-GET), never as a
// generic replay mechanism for newOrder, finalize or account mutations.
func (c *Client) retryRead(ctx context.Context, method, url string, request func() (http.Header, []byte, error)) (http.Header, []byte, error) {
	for attempt := 1; ; attempt++ {
		started := time.Now()
		headers, body, err := request()
		if err == nil || ctx.Err() != nil || !IsRetryable(err) || attempt >= c.RetryPolicy.Attempts() {
			return headers, body, err
		}
		delay := max(c.RetryPolicy.Delay(attempt), time.Until(RetryTime(err)))
		if c.Logger != nil {
			c.Logger.Warn("ACME request retry", "method", method, "url", url, "attempt", attempt, "elapsed", time.Since(started), "delay", delay, "error", err)
		}
		if waitErr := wait(ctx, delay); waitErr != nil {
			// Preserve a rate limit's Retry-After even when the stage budget
			// expires while waiting; the issuance scheduler must honor it too.
			return headers, body, errors.Join(err, waitErr)
		}
	}
}

// HTTPOptions configures the default ACME transport. Zero values select a
// 60-second request timeout and a 30-second TLS handshake timeout.
type HTTPOptions struct {
	Timeout             time.Duration
	TLSHandshakeTimeout time.Duration
}

// NewHTTPClient creates a connection-pooling client without modifying the
// process-wide http.DefaultTransport. The caller owns its idle connections.
func NewHTTPClient(options HTTPOptions) *http.Client {
	if options.Timeout <= 0 {
		options.Timeout = time.Minute
	}
	if options.TLSHandshakeTimeout <= 0 {
		options.TLSHandshakeTimeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = options.TLSHandshakeTimeout
	return &http.Client{Transport: transport, Timeout: options.Timeout}
}

var defaultHTTPClient = NewHTTPClient(HTTPOptions{})
