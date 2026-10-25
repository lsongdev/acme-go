package store

import (
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"testing"
)

// legacySchema is the schema before tables were prefixed for sharing a
// database, when the version lived in PRAGMA user_version.
//
//go:embed testdata/legacy.sql
var legacySchema string

func TestUpgradesUnprefixedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "acme.db")
	old, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		legacySchema,
		`INSERT INTO providers (name, directory_url, created_at) VALUES ('My CA', 'https://ca.example/dir', 1)`,
		`INSERT INTO accounts (provider_id, email, url, status, thumbprint, key_pem, created_at, updated_at) VALUES (1, 'a@b.c', 'https://ca.example/acct/1', 'valid', 't', 'k', 1, 1)`,
		`PRAGMA user_version = 1`,
	} {
		if _, err := old.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	providers, err := s.ListProviders(ctx)
	if err != nil || len(providers) != 1 || providers[0].Name != "My CA" {
		t.Fatalf("providers = %+v, %v (defaults must not be re-added)", providers, err)
	}
	accounts, err := s.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].ProviderName != "My CA" {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	// The foreign key followed the rename: the provider is still in use.
	if err := s.DeleteProvider(ctx, providers[0].ID); err == nil {
		t.Fatal("deleting a referenced provider succeeded")
	}
	var leftover int
	s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('providers', 'accounts', 'issuances_certificate')`).Scan(&leftover)
	if leftover != 0 {
		t.Fatalf("%d unprefixed objects remain", leftover)
	}
}

func TestSharesADatabase(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Another module's tables, and its own idea of user_version.
	db.Exec(`CREATE TABLE docker_apps (id TEXT PRIMARY KEY)`)
	db.Exec(`PRAGMA user_version = 7`)
	for range 2 { // migrating again is a no-op
		s, err := New(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if providers, _ := s.ListProviders(ctx); len(providers) != len(DefaultProviders) {
			t.Fatalf("providers = %d", len(providers))
		}
		s.Close() // must not close a database it does not own
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("shared database closed: %v", err)
	}
}
