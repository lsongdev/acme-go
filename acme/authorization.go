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

func (c *Client) GetAuthorization(authorizationURL string) (*Authorization, error) {
	_, data, err := c.postAsGet(authorizationURL)
	if err != nil {
		return nil, err
	}
	resp := &Authorization{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) GetAuthorizations(authorizationURLs []string) ([]*Authorization, error) {
	resp := make([]*Authorization, 0, len(authorizationURLs))
	for _, authorizationURL := range authorizationURLs {
		auth, err := c.GetAuthorization(authorizationURL)
		if err != nil {
			return nil, err
		}
		resp = append(resp, auth)
	}
	return resp, nil
}

func (c *Client) DeactivateAuthorization(authorizationURL string) error {
	_, _, err := c.post(authorizationURL, map[string]string{"status": "deactivated"})
	return err
}
