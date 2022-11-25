package acme

import (
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
	}
}

func (client *Client) GetChallenge(challengeUrl string) (resp *Challenge, err error) {
	_, data, err := client.get(challengeUrl)
	if err != nil {
		return
	}
	resp = &Challenge{}
	err = json.Unmarshal(data, &resp)
	return
}

func (client *Client) CompleteChallenge(challengeUrl string) (err error) {
	_, _, err = client.post(challengeUrl, nil)
	if err != nil {
		return
	}
	return
}

func (client *Client) GetKeyAuthorization(token string) (keyAuth string, err error) {
	thumbprint, err := client.GetThumbprint()
	keyAuth = token + "." + thumbprint
	return
}

type DNSRecord struct {
	Type    string
	Name    string
	Content string
}

func (client *Client) DNS01KeyAuthorization(token string) (record *DNSRecord, err error) {
	keyAuth, err := client.GetKeyAuthorization(token)
	hash := sha256.Sum256([]byte(keyAuth))
	record = &DNSRecord{
		Type:    "TXT",
		Name:    "_acme-challenge",
		Content: base64.RawURLEncoding.EncodeToString(hash[:]),
	}
	return
}

type File struct {
	FileName string
	Content  []byte
}

func (client *Client) HTTP01KeyAuthorization(token string) (file *File, err error) {
	keyAuth, err := client.GetKeyAuthorization(token)
	file = &File{
		Content:  []byte(keyAuth),
		FileName: fmt.Sprintf(".well-known/acme-challenge/%s", token),
	}
	return
}
