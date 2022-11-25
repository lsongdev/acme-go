package acme

import "encoding/json"

const AuthorizationStatusPending = "pending"
const AuthorizationStatusValid = "valid"

type Authorization struct {
	Status     string      `json:"status"`
	Expires    string      `json:"expires"`
	Identifier Identifier  `json:"identifier"`
	Challenges []Challenge `json:"challenges"`
}

func (client *Client) GetAuthorization(authorizationUrl string) (resp *Authorization, err error) {
	_, data, err := client.get(authorizationUrl)
	if err != nil {
		return
	}
	resp = &Authorization{}
	err = json.Unmarshal(data, &resp)
	return
}

func (client *Client) GetAuthorizations(authorizationUrls []string) (resp []*Authorization, err error) {
	for _, authorizationUrl := range authorizationUrls {
		auth, err := client.GetAuthorization(authorizationUrl)
		if err != nil {
			return nil, err
		}
		resp = append(resp, auth)
	}
	return
}

func (client *Client) CompleteAuthorization(authorizationUrl string) (err error) {
	_, _, err = client.post(authorizationUrl, nil)
	if err != nil {
		return
	}
	return
}

func (client *Client) DeactivateAuthorization(authorizationUrl string) (err error) {
	deactivateReq := map[string]interface{}{
		"status": "deactivated",
	}
	_, _, err = client.post(authorizationUrl, deactivateReq)
	if err != nil {
		return
	}
	return
}
