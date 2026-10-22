package acme

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultPollInterval = time.Second
	maxPollInterval     = time.Minute
)

// WaitAuthorization polls an authorization until it is no longer pending.
// It returns the authorization once valid, or an error carrying the failed
// challenge's problem when it becomes invalid.
func (c *Client) WaitAuthorization(ctx context.Context, authorizationURL string) (*Authorization, error) {
	authorization := &Authorization{}
	err := c.poll(ctx, authorizationURL, authorization, func() (bool, error) {
		switch authorization.Status {
		case AuthorizationStatusPending:
			return false, nil
		case AuthorizationStatusValid:
			return true, nil
		}
		for _, challenge := range authorization.Challenges {
			if challenge.Status == ChallengeStatusInvalid && challenge.Error.Type != "" {
				problem := challenge.Error
				return true, &problem
			}
		}
		return true, fmt.Errorf("acme: authorization for %s is %s", authorization.Identifier.Value, authorization.Status)
	})
	return authorization, err
}

// WaitOrder polls an order while it is pending or processing. It returns the
// order once ready (to be finalized) or valid (certificate issued), and an
// error when it becomes invalid.
func (c *Client) WaitOrder(ctx context.Context, orderURL string) (*OrderResponse, error) {
	order := &OrderResponse{}
	err := c.poll(ctx, orderURL, order, func() (bool, error) {
		switch order.Status {
		case OrderStatusPending, OrderStatusProcessing:
			return false, nil
		case OrderStatusReady, OrderStatusValid:
			return true, nil
		default:
			return true, fmt.Errorf("acme: order is %s", order.Status)
		}
	})
	return order, err
}

// poll fetches url with POST-as-GET into target until done reports true,
// waiting between attempts as the server's Retry-After header asks.
func (c *Client) poll(ctx context.Context, url string, target any, done func() (bool, error)) error {
	for {
		headers, data, err := c.postAsGet(ctx, url)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, target); err != nil {
			return err
		}
		if finished, err := done(); finished || err != nil {
			return err
		}
		timer := time.NewTimer(retryAfter(headers, defaultPollInterval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// retryAfter parses a Retry-After header given in seconds or as an HTTP date.
func retryAfter(headers http.Header, fallback time.Duration) time.Duration {
	value := headers.Get("Retry-After")
	delay := fallback
	if seconds, err := strconv.Atoi(value); err == nil {
		delay = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(value); err == nil {
		delay = time.Until(date)
	}
	return min(max(delay, 0), maxPollInterval)
}
