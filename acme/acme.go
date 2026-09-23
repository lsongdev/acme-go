package acme

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const badNonceError = "urn:ietf:params:acme:error:badNonce"


type Directory struct {
	NewNonce    string `json:"newNonce"`    // url to new nonce endpoint
	NewAccount  string `json:"newAccount"`  // url to new account endpoint
	NewOrder    string `json:"newOrder"`    // url to new order endpoint
	NewAuthz    string `json:"newAuthz"`    // url to new authz endpoint
	RevokeCert  string `json:"revokeCert"`  // url to revoke cert endpoint
	KeyChange   string `json:"keyChange"`   // url to key change endpoint
	RenewalInfo string `json:"renewalInfo"` // url to renewal info endpoint

	// meta object containing directory metadata
	Meta struct {
		TermsOfService          string   `json:"termsOfService"`
		Website                 string   `json:"website"`
		CaaIdentities           []string `json:"caaIdentities"`
		ExternalAccountRequired bool     `json:"externalAccountRequired"`
	} `json:"meta"`
}

type Response struct {
	Status int    `json:"status"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

func (r *Response) Error() string {
	if r.Type == "" {
		if r.Detail != "" {
			return r.Detail
		}
		return fmt.Sprintf("acme: HTTP %d", r.Status)
	}
	if r.Detail == "" {
		return r.Type
	}
	return fmt.Sprintf("%s: %s", r.Type, r.Detail)
}

type Config struct {
	AccountKey   string `json:"accountKey"`
	AccountURL   string `json:"accountURL"`
	DirectoryURL string `json:"directoryURL"`
}

type Client struct {
	Config     *Config
	Directory  *Directory
	PrivateKey crypto.Signer
	AccountURL string
	nonce      string
}

func NewDefaultConfig() *Config {
	return &Config{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
	}
}

func NewClient(config *Config) (client *Client, err error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	client = &Client{Config: config, AccountURL: config.AccountURL}
	if config.AccountKey != "" {
		if err = client.ImportKey(config.AccountKey); err != nil {
			return nil, err
		}
	}
	client.Directory, err = client.GetDirectory()
	return client, err
}

func (c *Client) request(method, url string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/jose+json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if nonce := res.Header.Get("Replay-Nonce"); nonce != "" {
		c.nonce = nonce
	}
	return res, nil
}

func (c *Client) GenerateKey() (err error) {
	c.PrivateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return err
}

func (c *Client) ImportKey(data string) error {
	block, _ := pem.Decode([]byte(data))
	if block == nil {
		return errors.New("acme: invalid PEM account key")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("acme: parse account key: %w", err)
	}
	c.PrivateKey = key
	return nil
}

func (c *Client) ExportKey() (string, error) {
	key, ok := c.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return "", ErrUnsupportedKey
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})), nil
}

func (c *Client) GetThumbprint() (string, error) {
	if c.PrivateKey == nil {
		return "", errors.New("acme: account key is not configured")
	}
	return JWKThumbprint(c.PrivateKey.Public())
}

func readResponse(res *http.Response) (http.Header, []byte, error) {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return res.Header, nil, err
	}
	if res.StatusCode < http.StatusBadRequest {
		return res.Header, body, nil
	}

	problem := &Response{Status: res.StatusCode}
	if err := json.Unmarshal(body, problem); err != nil {
		return res.Header, body, fmt.Errorf("acme: %s", res.Status)
	}
	if problem.Status == 0 {
		problem.Status = res.StatusCode
	}
	return res.Header, body, problem
}

func (c *Client) get(url string) (http.Header, []byte, error) {
	res, err := c.request(http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	return readResponse(res)
}

func (c *Client) post(url string, payload interface{}) (http.Header, []byte, error) {
	return c.postWithKID(url, payload, c.AccountURL)
}

func (c *Client) postJWK(url string, payload interface{}) (http.Header, []byte, error) {
	return c.postWithKID(url, payload, "")
}

func (c *Client) postWithKID(url string, payload interface{}, kid string) (http.Header, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return c.postRaw(url, body, kid)
}

func (c *Client) postAsGet(url string) (http.Header, []byte, error) {
	return c.postRaw(url, nil, c.AccountURL)
}

func (c *Client) postRaw(url string, payload []byte, kid string) (http.Header, []byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		data, err := c.buildSignedRequestData(url, payload, kid)
		if err != nil {
			return nil, nil, err
		}
		res, err := c.request(http.MethodPost, url, data)
		if err != nil {
			return nil, nil, err
		}
		headers, body, err := readResponse(res)
		if err == nil {
			return headers, body, nil
		}
		var problem *Response
		if errors.As(err, &problem) && problem.Type == badNonceError && attempt == 0 {
			continue
		}
		return headers, body, err
	}
	return nil, nil, errors.New("acme: request failed")
}

func (c *Client) GetDirectory() (*Directory, error) {
	_, data, err := c.get(c.Config.DirectoryURL)
	if err != nil {
		return nil, err
	}
	directory := &Directory{}
	if err := json.Unmarshal(data, directory); err != nil {
		return nil, err
	}
	return directory, nil
}

func (c *Client) getNonce() (string, error) {
	if c.nonce != "" {
		nonce := c.nonce
		c.nonce = ""
		return nonce, nil
	}
	if c.Directory == nil || c.Directory.NewNonce == "" {
		return "", errors.New("acme: newNonce endpoint is not configured")
	}
	res, err := c.request(http.MethodHead, c.Directory.NewNonce, nil)
	if err != nil {
		return "", err
	}
	res.Body.Close()
	if res.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("acme: nonce request failed: %s", res.Status)
	}
	if c.nonce == "" {
		return "", errors.New("acme: nonce response missing Replay-Nonce header")
	}
	nonce := c.nonce
	c.nonce = ""
	return nonce, nil
}
