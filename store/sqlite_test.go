package store

import (
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"strings"
	"testing"
)

// legacySchema is the schema before tables were prefixed for sharing a
// database, when the version lived in PRAGMA user_version.
//
//go:embed testdata/legacy.sql
var legacySchema string

//go:embed testdata/original.sql
var originalSchema string

func TestUpgradesOriginalProviderOnlyDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "acme.db")
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		originalSchema,
		`INSERT INTO acme_providers (id, name, directory_url, created_at) VALUES (42, 'My CA', 'https://ca.example/dir', '2026-10-25 08:00:00')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		providers, err := s.ListProviders(ctx)
		s.Close()
		if err != nil || len(providers) != 1 {
			t.Fatalf("providers = %+v, %v", providers, err)
		}
		p := providers[0]
		if p.ID != 42 || p.Name != "My CA" || p.DirectoryURL != "https://ca.example/dir" || p.CreatedAt.UTC().Format("2006-01-02 15:04:05") != "2026-10-25 08:00:00" {
			t.Fatalf("provider changed: %+v", p)
		}
	}
	var backups, version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name GLOB 'acme_legacy_*'`).Scan(&backups); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT version FROM acme_schema`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if backups != 7 || version != len(migrations) {
		t.Fatalf("backups = %d, version = %d", backups, version)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM acme_legacy_providers WHERE id = 42`).Scan(&name); err != nil || name != "My CA" {
		t.Fatalf("archived provider = %q, %v", name, err)
	}
}

func TestOriginalDatabaseWithRecordsIsUnchanged(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "acme.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		originalSchema,
		`INSERT INTO certificates (domains, csr_pem, private_key_pem) VALUES ('example.com', 'csr', 'key')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(context.Background(), db); err == nil || !strings.Contains(err.Error(), "original database contains records in certificates") {
		t.Fatalf("upgrade error = %v", err)
	}
	var key string
	if err := db.QueryRow(`SELECT private_key_pem FROM certificates`).Scan(&key); err != nil || key != "key" {
		t.Fatalf("original certificate key = %q, %v", key, err)
	}
	var changed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'acme_schema' OR name GLOB 'acme_legacy_*'`).Scan(&changed); err != nil || changed != 0 {
		t.Fatalf("migration left %d objects, %v", changed, err)
	}
}

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
