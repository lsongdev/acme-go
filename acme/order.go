package acme

import (
	"encoding/base64"
	"encoding/json"
)

const OrderStatusPending = "pending"
const OrderStatusReady = "ready"
const OrderStatusProcessing = "processing"
const OrderStatusValid = "valid"
const OrderStatusInvalid = "invalid"

// Identifier object used in order and authorization objects
// See https://tools.ietf.org/html/rfc8555#section-7.1.4
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

func (client *Client) CreateOrder(request *OrderRequest) (url string, resp *OrderResponse, err error) {
	headers, data, err := client.post(client.Directory.NewOrder, request)
	if err != nil {
		return
	}
	url = headers.Get("Location")
	resp = &OrderResponse{}
	err = json.Unmarshal(data, &resp)
	return
}

func (client *Client) GetOrder(orderUrl string) (resp *OrderResponse, err error) {
	_, data, err := client.get(orderUrl)
	if err != nil {
		return
	}
	resp = &OrderResponse{}
	err = json.Unmarshal(data, &resp)
	return
}

func (client *Client) FinalizeOrder(finalizeUrl string, csrDER []byte) (resp *OrderResponse, err error) {
	finalizeReq := map[string]interface{}{
		"csr": base64.RawURLEncoding.EncodeToString(csrDER),
	}

	_, data, err := client.post(finalizeUrl, finalizeReq)
	if err != nil {
		return
	}
	resp = &OrderResponse{}
	err = json.Unmarshal(data, &resp)
	return
}
