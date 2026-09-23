package acme

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const ChallengeStatusPending = "pending"
const ChallengeStatusProcessing = "processing"
const ChallengeStatusValid = "valid"
const ChallengeStatusInvalid = "invalid"

type Challenge struct {
	Type             string    `json:"type"`
	Status           string    `json:"status"`
	URL              string    `json:"url"`
	Token            string    `json:"token"`
	Error            Response  `json:"error"`
	Validated        time.Time `json:"validated"`
	ValidationRecord []struct {
		HostName string `json:"hostname"`
	} `json:"validationRecord"`
}

func (c *Client) GetChallenge(ctx context.Context, challengeURL string) (*Challenge, error) {
	_, data, err := c.postAsGet(ctx, challengeURL)
	if err != nil {
		return nil, err
	}
	resp := &Challenge{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) CompleteChallenge(ctx context.Context, challengeURL string) error {
	_, _, err := c.post(ctx, challengeURL, struct{}{})
	return err
}

func (c *Client) GetKeyAuthorization(token string) (string, error) {
	thumbprint, err := c.GetThumbprint()
	if err != nil {
		return "", err
	}
	return token + "." + thumbprint, nil
}

type DNSRecord struct {
	Type    string
	Name    string
	Content string
}

func (c *Client) DNS01KeyAuthorization(token string) (*DNSRecord, error) {
	keyAuth, err := c.GetKeyAuthorization(token)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(keyAuth))
	return &DNSRecord{
		Type:    "TXT",
		Name:    "_acme-challenge",
		Content: base64.RawURLEncoding.EncodeToString(hash[:]),
	}, nil
}

type File struct {
	FileName string
	Content  []byte
}

func (c *Client) HTTP01KeyAuthorization(token string) (*File, error) {
	keyAuth, err := c.GetKeyAuthorization(token)
	if err != nil {
		return nil, err
	}
	return &File{
		Content:  []byte(keyAuth),
		FileName: fmt.Sprintf(".well-known/acme-challenge/%s", token),
	}, nil
}
