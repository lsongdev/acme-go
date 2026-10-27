package acme

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const OrderStatusPending = "pending"
const OrderStatusReady = "ready"
const OrderStatusProcessing = "processing"
const OrderStatusValid = "valid"
const OrderStatusInvalid = "invalid"

type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type OrderRequest struct {
	Identifiers []Identifier `json:"identifiers"`
}

type OrderResponse struct {
	Status         string       `json:"status"`
	Expires        string       `json:"expires"`
	Identifiers    []Identifier `json:"identifiers"`
	Authorizations []string     `json:"authorizations"`
	Finalize       string       `json:"finalize"`
	Certificate    string       `json:"certificate"`
}

func (c *Client) CreateOrder(ctx context.Context, request *OrderRequest) (string, *OrderResponse, error) {
	directory, err := c.directory(ctx)
	if err != nil {
		return "", nil, err
	}

	headers, data, err := c.post(ctx, directory.NewOrder, request)
	if err != nil {
		return "", nil, err
	}
	url := headers.Get("Location")
	resp := &OrderResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return "", nil, err
	}
	return url, resp, nil
}

func (c *Client) GetOrder(ctx context.Context, orderURL string) (*OrderResponse, error) {
	_, data, err := c.postAsGet(ctx, orderURL)
	if err != nil {
		return nil, err
	}
	resp := &OrderResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) FinalizeOrder(ctx context.Context, finalizeURL string, csrDER []byte) (*OrderResponse, error) {
	_, data, err := c.post(ctx, finalizeURL, map[string]string{
		"csr": base64.RawURLEncoding.EncodeToString(csrDER),
	})
	if err != nil {
		return nil, err
	}
	resp := &OrderResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// FinalizeAndWait reconciles the order before submitting a CSR. In particular,
// a lost finalize response does not imply failure: processing orders are polled
// and valid orders are returned without resubmitting. Retries use the same CSR.
func (c *Client) FinalizeAndWait(ctx context.Context, orderURL string, csrDER []byte) (*OrderResponse, error) {
	var lastErr error
	for attempts := 0; ; {
		order, err := c.WaitOrder(ctx, orderURL)
		if err != nil {
			return nil, err
		}
		if order.Status == OrderStatusValid {
			return order, nil
		}
		if attempts >= c.RetryPolicy.Attempts() {
			return nil, lastErr
		}
		attempts++
		result, err := c.FinalizeOrder(ctx, order.Finalize, csrDER)
		if err == nil && result.Status == OrderStatusValid {
			return result, nil
		}
		var problem *Response
		notReady := errors.As(err, &problem) && problem.Type == "urn:ietf:params:acme:error:orderNotReady"
		if err != nil && !IsRetryable(err) && !notReady {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		if lastErr == nil {
			lastErr = fmt.Errorf("acme: order remained %s after finalization", result.Status)
		}
		delay := max(c.RetryPolicy.Delay(attempts), time.Until(RetryTime(err)))
		if waitErr := wait(ctx, delay); waitErr != nil {
			return nil, errors.Join(lastErr, waitErr)
		}
	}
}
