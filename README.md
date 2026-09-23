# acme-go

A small ACME v2 client for Go.

`acme-go` intentionally keeps the protocol surface close to RFC 8555: account, order, authorization, challenge, certificate, JWS, and nonce handling, without adding a framework around them.

## Install

```sh
go get github.com/lsongdev/acme-go/acme
```

## Design

The package keeps persistent account state separate from runtime dependencies:

```go
type Config struct {
    AccountKey   string
    AccountURL   string
    DirectoryURL string
}
```

`Config` can be serialized and restored later to reuse an existing ACME account.

Runtime-only configuration belongs to `Client`:

```go
client.HTTPClient = customHTTPClient
```

`NewClient` performs no network I/O. The ACME directory is fetched lazily on the first operation that needs it.

All network operations accept `context.Context`; local cryptographic helpers do not.

## Quick start

```go
package main

import (
    "context"
    "log"

    "github.com/lsongdev/acme-go/acme"
)

func main() {
    ctx := context.Background()

    config := &acme.Config{
        DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
    }

    client, err := acme.NewClient(config)
    if err != nil {
        log.Fatal(err)
    }

    if err := client.GenerateKey(); err != nil {
        log.Fatal(err)
    }

    accountURL, _, err := client.Register(ctx, &acme.AccountRequest{
        TermsOfServiceAgreed: true,
        Contact: []string{"mailto:admin@example.com"},
    })
    if err != nil {
        log.Fatal(err)
    }

    accountKey, err := client.ExportKey()
    if err != nil {
        log.Fatal(err)
    }

    // Persist these values and pass them to NewClient next time.
    config.AccountURL = accountURL
    config.AccountKey = accountKey
}
```

To restore an existing account:

```go
client, err := acme.NewClient(&acme.Config{
    DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
    AccountURL:   savedAccountURL,
    AccountKey:   savedAccountKey,
})
```

## Account keys

The client uses `crypto.Signer` internally.

It accepts existing:

- PKCS#8 private keys (`PRIVATE KEY`)
- SEC 1 EC private keys (`EC PRIVATE KEY`)
- PKCS#1 RSA private keys (`RSA PRIVATE KEY`)

Both RSA and ECDSA account keys are supported by the JWS layer. `ExportKey` always emits PKCS#8 so persistence uses one format regardless of key type.

`GenerateKey` currently creates a P-256 ECDSA key.

## Custom HTTP client

Set `Client.HTTPClient` when you need custom timeouts, proxies, TLS settings, transports, or test doubles:

```go
client.HTTPClient = &http.Client{
    Timeout: 30 * time.Second,
}
```

If it is nil, `http.DefaultClient` is used.

## Orders and challenges

Create an order:

```go
orderURL, order, err := client.CreateOrder(ctx, &acme.OrderRequest{
    Identifiers: []acme.Identifier{
        {Type: "dns", Value: "example.com"},
    },
})
```

Fetch ACME resources with POST-as-GET:

```go
order, err := client.GetOrder(ctx, orderURL)
authorization, err := client.GetAuthorization(ctx, order.Authorizations[0])
```

For DNS-01:

```go
record, err := client.DNS01KeyAuthorization(challenge.Token)
// record.Name    == "_acme-challenge"
// record.Type    == "TXT"
// record.Content == value expected by the ACME server
```

For HTTP-01:

```go
file, err := client.HTTP01KeyAuthorization(challenge.Token)
// serve file.Content at file.FileName
```

After provisioning the challenge:

```go
err := client.CompleteChallenge(ctx, challenge.URL)
```

## Scope

The package aims to stay small. It does not provision DNS records, run HTTP servers, manage certificate storage, or implement renewal scheduling. Those responsibilities can be composed around the ACME protocol client.
