package acme

import (
	"bytes"
	"context"
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
	NewNonce    string `json:"newNonce"`
	NewAccount  string `json:"newAccount"`
	NewOrder    string `json:"newOrder"`
	NewAuthz    string `json:"newAuthz"`
	RevokeCert  string `json:"revokeCert"`
	KeyChange   string `json:"keyChange"`
	RenewalInfo string `json:"renewalInfo"`

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

// Config is serializable ACME account state.
// HTTPClient belongs to Client because it is a runtime dependency, not account state.
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
	HTTPClient *http.Client
	nonce      string
}

func NewDefaultConfig() *Config {
	return &Config{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
	}
}

// NewClient performs no network I/O. The ACME directory is loaded lazily.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	client := &Client{
		Config:     config,
		AccountURL: config.AccountURL,
	}
	if config.AccountKey != "" {
		if err := client.ImportKey(config.AccountKey); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) request(ctx context.Context, method, url string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/jose+json")
	}

	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if nonce := res.Header.Get("Replay-Nonce"); nonce != "" {
		c.nonce = nonce
	}
	return res, nil
}

func (c *Client) GenerateKey() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	c.PrivateKey = key
	return nil
}

func (c *Client) ImportKey(data string) error {
	block, _ := pem.Decode([]byte(data))
	if block == nil {
		return errors.New("acme: invalid PEM account key")
	}

	key, err := parsePrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	if alg, _ := jwsHasher(key.Public()); alg == "" {
		return ErrUnsupportedKey
	}
	c.PrivateKey = key
	return nil
}

func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			return signer, nil
		}
		return nil, ErrUnsupportedKey
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("acme: unsupported private key format")
}

func (c *Client) ExportKey() (string, error) {
	if c.PrivateKey == nil {
		return "", errors.New("acme: account key is not configured")
	}
	if alg, _ := jwsHasher(c.PrivateKey.Public()); alg == "" {
		return "", ErrUnsupportedKey
	}

	der, err := x509.MarshalPKCS8PrivateKey(c.PrivateKey)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
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

func (c *Client) get(ctx context.Context, url string) (http.Header, []byte, error) {
	res, err := c.request(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	return readResponse(res)
}

func (c *Client) post(ctx context.Context, url string, payload any) (http.Header, []byte, error) {
	return c.postWithKID(ctx, url, payload, c.AccountURL)
}

func (c *Client) postJWK(ctx context.Context, url string, payload any) (http.Header, []byte, error) {
	return c.postWithKID(ctx, url, payload, "")
}

func (c *Client) postWithKID(ctx context.Context, url string, payload any, kid string) (http.Header, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return c.postRaw(ctx, url, body, kid)
}

func (c *Client) postAsGet(ctx context.Context, url string) (http.Header, []byte, error) {
	return c.postRaw(ctx, url, nil, c.AccountURL)
}

func (c *Client) postRaw(ctx context.Context, url string, payload []byte, kid string) (http.Header, []byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		data, err := c.buildSignedRequestData(ctx, url, payload, kid)
		if err != nil {
			return nil, nil, err
		}
		res, err := c.request(ctx, http.MethodPost, url, data)
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

func (c *Client) buildSignedRequestData(ctx context.Context, url string, payload []byte, kid string) ([]byte, error) {
	nonce, err := c.getNonce(ctx)
	if err != nil {
		return nil, err
	}
	return jwsEncodeJSON(payload, c.PrivateKey, kid, nonce, url)
}

// GetDirectory fetches and caches the ACME directory.
func (c *Client) GetDirectory(ctx context.Context) (*Directory, error) {
	if c.Config == nil || c.Config.DirectoryURL == "" {
		return nil, errors.New("acme: directory URL is not configured")
	}
	_, data, err := c.get(ctx, c.Config.DirectoryURL)
	if err != nil {
		return nil, err
	}
	directory := &Directory{}
	if err := json.Unmarshal(data, directory); err != nil {
		return nil, err
	}
	c.Directory = directory
	return directory, nil
}

func (c *Client) directory(ctx context.Context) (*Directory, error) {
	if c.Directory != nil {
		return c.Directory, nil
	}
	return c.GetDirectory(ctx)
}

func (c *Client) getNonce(ctx context.Context) (string, error) {
	if c.nonce != "" {
		nonce := c.nonce
		c.nonce = ""
		return nonce, nil
	}

	directory, err := c.directory(ctx)
	if err != nil {
		return "", err
	}
	if directory.NewNonce == "" {
		return "", errors.New("acme: newNonce endpoint is not configured")
	}

	res, err := c.request(ctx, http.MethodHead, directory.NewNonce, nil)
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
