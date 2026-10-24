// Package web assembles the certificate manager as a panel module: the
// issuance service with its JSON API, its SQLite store, and the browser UI
// (native ES modules in ui/, no build step).
//
// An ACME value satisfies the panel module contract without depending on
// any panel: Handler serves the API with relative routes and Assets holds
// the UI with manifest.json at its root. The standalone command and a panel
// build it the same way, through New.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/lsongdev/acme-go/api"
	"github.com/lsongdev/acme-go/store"
)

//go:embed ui
var embedded embed.FS

// UI is the module's browser assets; manifest.json describes them.
var UI = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}()

// Options configures the certificate manager.
type Options struct {
	RenewBefore time.Duration // renew this long before expiry (30 days by default)
	DNSProvider api.DNSProvider
	Logger      *slog.Logger
}

// ACME is an assembled certificate manager.
type ACME struct {
	Service *api.Service
}

// New migrates the certificate manager's tables in db (prefixed "acme_",
// so db may be shared with other modules) and starts the issuance service.
// The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*ACME, error) {
	certs, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(ctx, certs, opts)
}

// NewWithStore starts the issuance service on any api.Store.
func NewWithStore(ctx context.Context, certs api.Store, opts Options) (*ACME, error) {
	service, err := api.NewService(ctx, certs, api.Options{RenewBefore: opts.RenewBefore, DNSProvider: opts.DNSProvider, Logger: opts.Logger})
	if err != nil {
		return nil, err
	}
	return &ACME{Service: service}, nil
}

// Handler serves the module's API with relative routes.
func (a *ACME) Handler() http.Handler { return a.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (a *ACME) Assets() fs.FS { return UI }

// Close stops running issuances. The database belongs to the caller.
func (a *ACME) Close() error { return a.Service.Close() }
