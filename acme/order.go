package acme

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
