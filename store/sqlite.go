// Package store implements api.Store on SQLite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/acme-go/api"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SQLite is an api.Store backed by a SQLite database file.
type SQLite struct {
	db *sql.DB
}

var _ api.Store = (*SQLite)(nil)

// DefaultProviders are added to a new database.
var DefaultProviders = []api.Provider{
	{Name: "Let's Encrypt", DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory"},
	{Name: "Let's Encrypt Staging", DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"},
}

// Open opens (creating if needed) the database at path and migrates it.
func Open(ctx context.Context, path string) (*SQLite, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serializes writers and keeps the pragmas in effect.
	db.SetMaxOpenConns(1)
	store := &SQLite{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return store, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

// migrations are applied in order; the database's user_version records how
// many have run. Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE providers (
	id            INTEGER PRIMARY KEY,
	name          TEXT NOT NULL UNIQUE,
	directory_url TEXT NOT NULL UNIQUE,
	created_at    INTEGER NOT NULL
);
CREATE TABLE accounts (
	id          INTEGER PRIMARY KEY,
	provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
	email       TEXT NOT NULL,
	url         TEXT NOT NULL UNIQUE,
	status      TEXT NOT NULL,
	thumbprint  TEXT NOT NULL,
	key_pem     TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE TABLE certificates (
	id             INTEGER PRIMARY KEY,
	domains        TEXT NOT NULL,
	account_id     INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
	challenge_type TEXT NOT NULL,
	auto_renew     INTEGER NOT NULL,
	status         TEXT NOT NULL,
	cert_pem       TEXT NOT NULL DEFAULT '',
	key_pem        TEXT NOT NULL DEFAULT '',
	issuer         TEXT NOT NULL DEFAULT '',
	serial_number  TEXT NOT NULL DEFAULT '',
	not_before     INTEGER NOT NULL DEFAULT 0,
	not_after      INTEGER NOT NULL DEFAULT 0,
	created_at     INTEGER NOT NULL,
	updated_at     INTEGER NOT NULL
);
CREATE TABLE issuances (
	id             INTEGER PRIMARY KEY,
	certificate_id INTEGER NOT NULL REFERENCES certificates(id) ON DELETE CASCADE,
	account_id     INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
	domains        TEXT NOT NULL,
	challenge_type TEXT NOT NULL,
	status         TEXT NOT NULL,
	step           TEXT NOT NULL,
	error          TEXT NOT NULL DEFAULT '',
	dns_records    TEXT NOT NULL DEFAULT '[]',
	csr_pem        TEXT NOT NULL DEFAULT '',
	key_pem        TEXT NOT NULL DEFAULT '',
	created_at     INTEGER NOT NULL,
	updated_at     INTEGER NOT NULL
);
CREATE INDEX issuances_certificate ON issuances(certificate_id);
-- At most one unfinished issuance per certificate.
CREATE UNIQUE INDEX issuances_unfinished ON issuances(certificate_id) WHERE status NOT IN ('succeeded', 'failed');
CREATE TABLE orders (
	id          INTEGER PRIMARY KEY,
	account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	issuance_id INTEGER REFERENCES issuances(id) ON DELETE SET NULL,
	url         TEXT NOT NULL UNIQUE,
	domains     TEXT NOT NULL,
	status      TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this program supports (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	if version == 0 {
		now := time.Now().Unix()
		for _, provider := range DefaultProviders {
			if _, err := tx.ExecContext(ctx, `INSERT INTO providers (name, directory_url, created_at) VALUES (?, ?, ?)`, provider.Name, provider.DirectoryURL, now); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations))); err != nil {
		return err
	}
	return tx.Commit()
}

// mapError translates driver errors into api errors.
func mapError(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %w", what, api.ErrNotFound)
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch code := sqliteErr.Code(); {
		case code == sqlite3.SQLITE_CONSTRAINT_UNIQUE, code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			return fmt.Errorf("%w: %s already exists", api.ErrConflict, what)
		// ON DELETE RESTRICT violations are reported as
		// SQLITE_CONSTRAINT_TRIGGER, so match on the message instead.
		case code&0xff == sqlite3.SQLITE_CONSTRAINT && strings.Contains(err.Error(), "FOREIGN KEY"):
			return fmt.Errorf("%w: %s is still in use or references a missing record", api.ErrConflict, what)
		}
	}
	return err
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(seconds int64) time.Time {
	if seconds == 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func joinDomains(domains []string) string { return strings.Join(domains, ",") }

func splitDomains(value string) []string {
	if value == "" {
		return []string{}
	}
	return strings.Split(value, ",")
}

// checkAffected turns an UPDATE or DELETE that matched nothing into
// ErrNotFound.
func checkAffected(result sql.Result, err error, what string) error {
	if err != nil {
		return mapError(err, what)
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%s %w", what, api.ErrNotFound)
	}
	return nil
}

type scanner interface{ Scan(...any) error }

// Providers

const providerColumns = `id, name, directory_url, created_at`

func scanProvider(row scanner) (api.Provider, error) {
	var p api.Provider
	var created int64
	err := row.Scan(&p.ID, &p.Name, &p.DirectoryURL, &created)
	p.CreatedAt = fromUnix(created)
	return p, err
}

func (s *SQLite) ListProviders(ctx context.Context) ([]api.Provider, error) {
	return list(ctx, s.db, scanProvider, `SELECT `+providerColumns+` FROM providers ORDER BY id`)
}

func (s *SQLite) GetProvider(ctx context.Context, id int64) (*api.Provider, error) {
	p, err := scanProvider(s.db.QueryRowContext(ctx, `SELECT `+providerColumns+` FROM providers WHERE id = ?`, id))
	if err != nil {
		return nil, mapError(err, "provider")
	}
	return &p, nil
}

func (s *SQLite) CreateProvider(ctx context.Context, p *api.Provider) error {
	p.CreatedAt = time.Now().UTC().Truncate(time.Second)
	result, err := s.db.ExecContext(ctx, `INSERT INTO providers (name, directory_url, created_at) VALUES (?, ?, ?)`, p.Name, p.DirectoryURL, unix(p.CreatedAt))
	if err != nil {
		return mapError(err, "provider")
	}
	p.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) DeleteProvider(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM providers WHERE id = ?`, id)
	return checkAffected(result, err, "provider")
}

// Accounts

const accountColumns = `a.id, a.provider_id, p.name, a.email, a.url, a.status, a.thumbprint, a.key_pem, a.created_at, a.updated_at`
const accountFrom = ` FROM accounts a JOIN providers p ON p.id = a.provider_id`

func scanAccount(row scanner) (api.Account, error) {
	var a api.Account
	var created, updated int64
	err := row.Scan(&a.ID, &a.ProviderID, &a.ProviderName, &a.Email, &a.URL, &a.Status, &a.Thumbprint, &a.KeyPEM, &created, &updated)
	a.CreatedAt, a.UpdatedAt = fromUnix(created), fromUnix(updated)
	return a, err
}

func (s *SQLite) ListAccounts(ctx context.Context) ([]api.Account, error) {
	return list(ctx, s.db, scanAccount, `SELECT `+accountColumns+accountFrom+` ORDER BY a.id`)
}

func (s *SQLite) GetAccount(ctx context.Context, id int64) (*api.Account, error) {
	a, err := scanAccount(s.db.QueryRowContext(ctx, `SELECT `+accountColumns+accountFrom+` WHERE a.id = ?`, id))
	if err != nil {
		return nil, mapError(err, "account")
	}
	return &a, nil
}

func (s *SQLite) CreateAccount(ctx context.Context, a *api.Account) error {
	now := time.Now().UTC().Truncate(time.Second)
	a.CreatedAt, a.UpdatedAt = now, now
	result, err := s.db.ExecContext(ctx, `INSERT INTO accounts (provider_id, email, url, status, thumbprint, key_pem, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ProviderID, a.Email, a.URL, a.Status, a.Thumbprint, a.KeyPEM, unix(now), unix(now))
	if err != nil {
		return mapError(err, "account")
	}
	a.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) UpdateAccountStatus(ctx context.Context, id int64, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE accounts SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), id)
	return checkAffected(result, err, "account")
}

func (s *SQLite) DeleteAccount(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	return checkAffected(result, err, "account")
}

// Certificates

const certificateColumns = `id, domains, account_id, challenge_type, auto_renew, status, cert_pem, key_pem, issuer, serial_number, not_before, not_after, created_at, updated_at`

func scanCertificate(row scanner) (api.Certificate, error) {
	var c api.Certificate
	var domains string
	var notBefore, notAfter, created, updated int64
	err := row.Scan(&c.ID, &domains, &c.AccountID, &c.ChallengeType, &c.AutoRenew, &c.Status, &c.CertPEM, &c.KeyPEM, &c.Issuer, &c.SerialNumber, &notBefore, &notAfter, &created, &updated)
	c.Domains = splitDomains(domains)
	c.NotBefore, c.NotAfter = fromUnix(notBefore), fromUnix(notAfter)
	c.CreatedAt, c.UpdatedAt = fromUnix(created), fromUnix(updated)
	return c, err
}

func (s *SQLite) ListCertificates(ctx context.Context) ([]api.Certificate, error) {
	return list(ctx, s.db, scanCertificate, `SELECT `+certificateColumns+` FROM certificates ORDER BY id DESC`)
}

func (s *SQLite) GetCertificate(ctx context.Context, id int64) (*api.Certificate, error) {
	c, err := scanCertificate(s.db.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificates WHERE id = ?`, id))
	if err != nil {
		return nil, mapError(err, "certificate")
	}
	return &c, nil
}

func (s *SQLite) CreateCertificate(ctx context.Context, c *api.Certificate) error {
	now := time.Now().UTC().Truncate(time.Second)
	c.CreatedAt, c.UpdatedAt = now, now
	result, err := s.db.ExecContext(ctx, `INSERT INTO certificates (domains, account_id, challenge_type, auto_renew, status, cert_pem, key_pem, issuer, serial_number, not_before, not_after, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		joinDomains(c.Domains), c.AccountID, c.ChallengeType, c.AutoRenew, c.Status, c.CertPEM, c.KeyPEM, c.Issuer, c.SerialNumber, unix(c.NotBefore), unix(c.NotAfter), unix(now), unix(now))
	if err != nil {
		return mapError(err, "certificate")
	}
	c.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) UpdateCertificate(ctx context.Context, c *api.Certificate) error {
	c.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	result, err := s.db.ExecContext(ctx, `UPDATE certificates SET domains = ?, account_id = ?, challenge_type = ?, auto_renew = ?, status = ?, cert_pem = ?, key_pem = ?, issuer = ?, serial_number = ?, not_before = ?, not_after = ?, updated_at = ? WHERE id = ?`,
		joinDomains(c.Domains), c.AccountID, c.ChallengeType, c.AutoRenew, c.Status, c.CertPEM, c.KeyPEM, c.Issuer, c.SerialNumber, unix(c.NotBefore), unix(c.NotAfter), unix(c.UpdatedAt), c.ID)
	return checkAffected(result, err, "certificate")
}

func (s *SQLite) DeleteCertificate(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM certificates WHERE id = ?`, id)
	return checkAffected(result, err, "certificate")
}

// Issuances

const issuanceColumns = `id, certificate_id, account_id, domains, challenge_type, status, step, error, dns_records, csr_pem, key_pem, created_at, updated_at`

func scanIssuance(row scanner) (api.Issuance, error) {
	var i api.Issuance
	var domains, records string
	var created, updated int64
	if err := row.Scan(&i.ID, &i.CertificateID, &i.AccountID, &domains, &i.ChallengeType, &i.Status, &i.Step, &i.Error, &records, &i.CSRPEM, &i.KeyPEM, &created, &updated); err != nil {
		return i, err
	}
	i.Domains = splitDomains(domains)
	i.CreatedAt, i.UpdatedAt = fromUnix(created), fromUnix(updated)
	return i, json.Unmarshal([]byte(records), &i.DNSRecords)
}

func (s *SQLite) ListIssuances(ctx context.Context, certificateID int64) ([]api.Issuance, error) {
	if certificateID > 0 {
		return list(ctx, s.db, scanIssuance, `SELECT `+issuanceColumns+` FROM issuances WHERE certificate_id = ? ORDER BY id DESC`, certificateID)
	}
	return list(ctx, s.db, scanIssuance, `SELECT `+issuanceColumns+` FROM issuances ORDER BY id DESC`)
}

func (s *SQLite) GetIssuance(ctx context.Context, id int64) (*api.Issuance, error) {
	i, err := scanIssuance(s.db.QueryRowContext(ctx, `SELECT `+issuanceColumns+` FROM issuances WHERE id = ?`, id))
	if err != nil {
		return nil, mapError(err, "issuance")
	}
	return &i, nil
}

func (s *SQLite) CreateIssuance(ctx context.Context, i *api.Issuance) error {
	now := time.Now().UTC().Truncate(time.Second)
	i.CreatedAt, i.UpdatedAt = now, now
	records, err := json.Marshal(nonNil(i.DNSRecords))
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO issuances (certificate_id, account_id, domains, challenge_type, status, step, error, dns_records, csr_pem, key_pem, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.CertificateID, i.AccountID, joinDomains(i.Domains), i.ChallengeType, i.Status, i.Step, i.Error, string(records), i.CSRPEM, i.KeyPEM, unix(now), unix(now))
	if err != nil {
		err = mapError(err, "issuance")
		if errors.Is(err, api.ErrConflict) {
			return fmt.Errorf("%w: certificate %d already has an issuance in progress", api.ErrConflict, i.CertificateID)
		}
		return err
	}
	i.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) UpdateIssuance(ctx context.Context, i *api.Issuance) error {
	i.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	records, err := json.Marshal(nonNil(i.DNSRecords))
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE issuances SET status = ?, step = ?, error = ?, dns_records = ?, csr_pem = ?, key_pem = ?, updated_at = ? WHERE id = ?`,
		i.Status, i.Step, i.Error, string(records), i.CSRPEM, i.KeyPEM, unix(i.UpdatedAt), i.ID)
	return checkAffected(result, err, "issuance")
}

func (s *SQLite) FailUnfinishedIssuances(ctx context.Context, reason string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	result, err := tx.ExecContext(ctx, `UPDATE issuances SET status = 'failed', error = ?, csr_pem = '', key_pem = '', updated_at = ? WHERE status NOT IN ('succeeded', 'failed')`, reason, now)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE certificates SET status = 'failed', updated_at = ? WHERE status = 'pending'`, now); err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// Orders

const orderColumns = `id, account_id, COALESCE(issuance_id, 0), url, domains, status, created_at, updated_at`

func scanOrder(row scanner) (api.Order, error) {
	var o api.Order
	var domains string
	var created, updated int64
	err := row.Scan(&o.ID, &o.AccountID, &o.IssuanceID, &o.URL, &domains, &o.Status, &created, &updated)
	o.Domains = splitDomains(domains)
	o.CreatedAt, o.UpdatedAt = fromUnix(created), fromUnix(updated)
	return o, err
}

func (s *SQLite) ListOrders(ctx context.Context) ([]api.Order, error) {
	return list(ctx, s.db, scanOrder, `SELECT `+orderColumns+` FROM orders ORDER BY id DESC`)
}

func (s *SQLite) CreateOrder(ctx context.Context, o *api.Order) error {
	now := time.Now().UTC().Truncate(time.Second)
	o.CreatedAt, o.UpdatedAt = now, now
	var issuanceID any
	if o.IssuanceID > 0 {
		issuanceID = o.IssuanceID
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO orders (account_id, issuance_id, url, domains, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		o.AccountID, issuanceID, o.URL, joinDomains(o.Domains), o.Status, unix(now), unix(now))
	if err != nil {
		return mapError(err, "order")
	}
	o.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) UpdateOrderStatus(ctx context.Context, id int64, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE orders SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), id)
	return checkAffected(result, err, "order")
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// list runs query and scans every row; it never returns a nil slice so the
// API encodes empty lists as [].
func list[T any](ctx context.Context, db *sql.DB, scan func(scanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
