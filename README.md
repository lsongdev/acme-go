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

After provisioning the challenge, ask the server to validate it and wait:

```go
err := client.CompleteChallenge(ctx, challenge.URL)
authorization, err := client.WaitAuthorization(ctx, order.Authorizations[0])
```

Once every authorization is valid, finalize with a CSR and wait for the
certificate:

```go
_, err = client.FinalizeOrder(ctx, order.Finalize, csrDER)
order, err = client.WaitOrder(ctx, orderURL)
chain, err := client.GetCertificatePEM(ctx, order.Certificate)
```

`WaitAuthorization` and `WaitOrder` honor the server's `Retry-After` header
and the context deadline. A `Client` is safe for concurrent use once its
fields are set.

## Scope

The package aims to stay small. It does not provision DNS records, run HTTP servers, manage certificate storage, or implement renewal scheduling. Those responsibilities can be composed around the ACME protocol client.

## Certificate manager

`cmd/acme` composes those responsibilities around the client: a small
certificate manager with a JSON API and a web UI, storing everything in
SQLite. The `acme` package itself stays dependency-free.

```sh
go run ./cmd/acme serve                       # UI on http://127.0.0.1:8080
ACME_PASSWORD=... go run ./cmd/acme serve -listen :8080 -http01 :80
```

- `api` — `Service` manages providers, accounts, certificates and issuances
  on top of the client, runs issuances in the background, and serves the JSON
  API (`Service.Handler`) and HTTP-01 responses (`Service.HTTP01Handler`).
  Storage is the `api.Store` interface.
- `store` — the SQLite `api.Store` (pure Go, no cgo).
- `store` tables are prefixed `acme_` and the schema version lives in
  `acme_schema`, so the database can be shared with other modules (a
  database from before this layout is upgraded in place).
- `web` — assembles the service, store and UI (`web.New(ctx, db, opts)`).
  `ACME` follows the panel module contract (`Handler`, `Assets`, a
  `manifest.json` and a `mount(el, ctx)` entry point), so a panel can mount
  it, and `Standalone` serves it on its own. The UI is Preact and htm ES
  modules served as-is, with no build step, vendored in `web/ui/vendor`.

HTTP-01 challenges are answered at `/.well-known/acme-challenge/` on the
UI listener and, with `-http01`, on a separate challenge-only listener; the
domain must resolve to this server on port 80. DNS-01 (required for wildcard
names) shows the TXT records to publish and waits for the operator to
continue; embedders can pass an `api.DNSProvider` to publish them
automatically. A server that owns port 80 itself (such as a reverse proxy)
can answer challenges with `Service.KeyAuthorization` and serve the issued
certificates from `Service.TLSCertificates`. Each issuance generates a new private key, and a failed
renewal leaves the current certificate in place. Certificates with
auto-renewal enabled are renewed 30 days before they expire.

The UI and API have no authentication unless `ACME_PASSWORD` is set (HTTP
basic auth, user `admin`), and they hold private keys, so keep the default
loopback address or set a password.
