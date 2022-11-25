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
	"fmt"
	"io"
	"net/http"
)

type H map[string]interface{}

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
		err = client.ImportKey(config.AccountKey)
		if err != nil {
			return
		}
	}
	client.Directory, err = client.GetDirectory()
	return
}

func (c *Client) request(method, url string, payload []byte) (res *http.Response, err error) {
	client := &http.Client{}
	req, err := http.NewRequest(method, url, bytes.NewBuffer(payload))
	if err != nil {
		return
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/jose+json")
	}
	res, err = client.Do(req)
	if err != nil {
		return
	}
	c.nonce = res.Header.Get("Replay-Nonce")
	return
}

func (acc *Client) GenerateKey() (err error) {
	acc.PrivateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return
}

func (acc *Client) ImportKey(data string) (err error) {
	b, _ := pem.Decode([]byte(data))
	key, err := x509.ParseECPrivateKey(b.Bytes)
	acc.PrivateKey = key
	return
}

func (acc *Client) ExportKey() (key string, err error) {
	certKeyEnc, err := x509.MarshalECPrivateKey(acc.PrivateKey.(*ecdsa.PrivateKey))
	data := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: certKeyEnc,
	})
	return string(data), err
}

func (acc *Client) GetThumbprint() (thumbprint string, err error) {
	return JWKThumbprint(acc.PrivateKey.Public())
}

func (client *Client) get(url string) (headers http.Header, body []byte, err error) {
	res, err := client.request(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	headers = res.Header
	defer res.Body.Close()
	body, err = io.ReadAll(res.Body)
	errResp := &Response{}
	json.Unmarshal(body, errResp)
	if errResp.Status != 0 {
		err = fmt.Errorf("%s: %s", errResp.Type, errResp.Detail)
	}
	return
}

func (client *Client) post(url string, payload interface{}) (headers http.Header, body []byte, err error) {
	data, err := client.buildSignedRequestData(url, payload)
	res, err := client.request(http.MethodPost, url, data)
	if err != nil {
		return
	}
	defer res.Body.Close()
	headers = res.Header
	body, err = io.ReadAll(res.Body)
	errResp := &Response{}
	json.Unmarshal(body, errResp)
	if errResp.Status != 0 {
		err = fmt.Errorf("%s: %s", errResp.Type, errResp.Detail)
	}
	return
}

func (client *Client) GetDirectory() (directory *Directory, err error) {
	_, data, err := client.get(client.Config.DirectoryURL)
	if err != nil {
		return
	}
	directory = &Directory{}
	err = json.Unmarshal(data, directory)
	return
}

func (client *Client) getNonce() (string, error) {
	if client.nonce != "" {
		return client.nonce, nil
	}
	_, err := client.request(http.MethodHead, client.Directory.NewNonce, nil)
	return client.nonce, err
}
