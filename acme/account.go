package acme

import (
	"encoding/json"
	"errors"
)

type AccountRequest struct {
	Contact              []string `json:"contact"`
	TermsOfServiceAgreed bool     `json:"termsOfServiceAgreed"`
}

type AccountResponse struct {
	Status    string   `json:"status"`
	Contact   []string `json:"contact"`
	InitialIP string   `json:"initialIp"`
	CreatedAt string   `json:"createdAt"`
	Key       struct {
		Kty string `json:"kty"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	} `json:"key"`
}

func (c *Client) Register(request *AccountRequest) (string, *AccountResponse, error) {
	headers, data, err := c.postJWK(c.Directory.NewAccount, request)
	if err != nil {
		return "", nil, err
	}
	url := headers.Get("Location")
	if url == "" {
		return "", nil, errors.New("acme: newAccount response missing Location header")
	}
	resp := &AccountResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return "", nil, err
	}
	c.AccountURL = url
	if c.Config != nil {
		c.Config.AccountURL = url
	}
	return url, resp, nil
}

func (c *Client) GetAccount(accountURL string) (*AccountResponse, error) {
	_, data, err := c.postAsGet(accountURL)
	if err != nil {
		return nil, err
	}
	resp := &AccountResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) DeactivateAccount(accountURL string) error {
	_, _, err := c.post(accountURL, map[string]string{"status": "deactivated"})
	return err
}
