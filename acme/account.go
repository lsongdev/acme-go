package acme

import (
	"encoding/json"
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

func (client *Client) Register(request *AccountRequest) (url string, resp *AccountResponse, err error) {
	headers, data, err := client.post(client.Directory.NewAccount, request)
	if err != nil {
		return
	}
	url = headers.Get("Location")
	resp = &AccountResponse{}
	err = json.Unmarshal(data, &resp)
	return
}

func (client *Client) GetAccount(accountUrl string) (resp *AccountResponse, err error) {
	_, data, err := client.post(accountUrl, nil)
	if err != nil {
		return
	}
	resp = &AccountResponse{}
	err = json.Unmarshal(data, &resp)
	return
}

func (client *Client) DeactivateAccount(accountUrl string) (err error) {
	deactivateReq := map[string]interface{}{
		"status": "deactivated",
	}
	client.post(accountUrl, deactivateReq)
	return
}
