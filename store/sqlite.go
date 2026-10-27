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

// SQLite is an api.Store on SQLite. Its tables are prefixed "acme_", so
// the database can be shared with other modules.
type SQLite struct {
	db    *sql.DB
	owned bool // opened by Open, so Close closes it
}

var _ api.Store = (*SQLite)(nil)

// DefaultProviders are added to a new database.
var DefaultProviders = []api.Provider{
	{Name: "Let's Encrypt", DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory"},
	{Name: "Let's Encrypt Staging", DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"},
}

// Open opens (creating if needed) a database file of its own at path and
// migrates it.
func Open(ctx context.Context, path string) (*SQLite, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serializes writers and keeps the pragmas in effect.
	db.SetMaxOpenConns(1)
	store, err := New(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.owned = true
	return store, nil
}

// New migrates the certificate manager's tables in db, which the caller
// owns and may share with other modules, and returns the store. db should
// enable foreign keys.
func New(ctx context.Context, db *sql.DB) (*SQLite, error) {
	store := &SQLite{db: db}
	if err := store.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return store, nil
}

// Close closes the database if Open opened it.
func (s *SQLite) Close() error {
	if !s.owned {
		return nil
	}
	return s.db.Close()
}

// migrations are applied in order; acme_schema records how many have run.
// Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE acme_providers (
	id            INTEGER PRIMARY KEY,
	name          TEXT NOT NULL UNIQUE,
	directory_url TEXT NOT NULL UNIQUE,
	created_at    INTEGER NOT NULL
);
CREATE TABLE acme_accounts (
	id          INTEGER PRIMARY KEY,
	provider_id INTEGER NOT NULL REFERENCES acme_providers(id) ON DELETE RESTRICT,
	email       TEXT NOT NULL,
	url         TEXT NOT NULL UNIQUE,
	status      TEXT NOT NULL,
	thumbprint  TEXT NOT NULL,
	key_pem     TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE TABLE acme_certificates (
	id             INTEGER PRIMARY KEY,
	domains        TEXT NOT NULL,
	account_id     INTEGER NOT NULL REFERENCES acme_accounts(id) ON DELETE RESTRICT,
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
CREATE TABLE acme_issuances (
	id             INTEGER PRIMARY KEY,
	certificate_id INTEGER NOT NULL REFERENCES acme_certificates(id) ON DELETE CASCADE,
	account_id     INTEGER NOT NULL REFERENCES acme_accounts(id) ON DELETE RESTRICT,
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
CREATE INDEX acme_issuances_certificate ON acme_issuances(certificate_id);
-- At most one unfinished issuance per certificate.
CREATE UNIQUE INDEX acme_issuances_unfinished ON acme_issuances(certificate_id) WHERE status NOT IN ('succeeded', 'failed');
CREATE TABLE acme_orders (
	id          INTEGER PRIMARY KEY,
	account_id  INTEGER NOT NULL REFERENCES acme_accounts(id) ON DELETE CASCADE,
	issuance_id INTEGER REFERENCES acme_issuances(id) ON DELETE SET NULL,
	url         TEXT NOT NULL UNIQUE,
	domains     TEXT NOT NULL,
	status      TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
`, `
ALTER TABLE acme_issuances ADD COLUMN dns_confirmed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE acme_issuances ADD COLUMN order_url TEXT NOT NULL DEFAULT '';
ALTER TABLE acme_issuances ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE acme_issuances ADD COLUMN next_attempt_at INTEGER NOT NULL DEFAULT 0;
-- Recover orders created before order_url was stored on the issuance.
UPDATE acme_issuances SET order_url = COALESCE((SELECT url FROM acme_orders WHERE issuance_id = acme_issuances.id ORDER BY id DESC LIMIT 1), '');
`}

// legacyRename upgrades a database from before the shared-database layout,
// whose tables had no prefix and whose version lived in user_version.
// Renaming keeps the data, and SQLite rewrites foreign keys to follow.
const legacyRename = `
ALTER TABLE providers RENAME TO acme_providers;
ALTER TABLE accounts RENAME TO acme_accounts;
ALTER TABLE certificates RENAME TO acme_certificates;
ALTER TABLE issuances RENAME TO acme_issuances;
ALTER TABLE orders RENAME TO acme_orders;
DROP INDEX issuances_certificate;
DROP INDEX issuances_unfinished;
CREATE INDEX acme_issuances_certificate ON acme_issuances(certificate_id);
CREATE UNIQUE INDEX acme_issuances_unfinished ON acme_issuances(certificate_id) WHERE status NOT IN ('succeeded', 'failed');
`

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS acme_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM acme_schema`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this program supports (%d)", version, len(migrations))
	}
	var original bool
	if version == 0 {
		original, err = archiveOriginalSchema(ctx, tx)
		if err != nil {
			return err
		}
		var legacy int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'providers'`).Scan(&legacy); err != nil {
			return err
		}
		if legacy > 0 {
			var userVersion int
			if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, legacyRename); err != nil {
				return fmt.Errorf("upgrade unprefixed tables: %w", err)
			}
			version = userVersion // the legacy migrations match these
		}
	}
	for i := version; i < len(migrations); i++ {
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	if original {
		if _, err := tx.ExecContext(ctx, `INSERT INTO acme_providers (id, name, directory_url, created_at)
SELECT id, name, directory_url, COALESCE(unixepoch(created_at), 0) FROM acme_legacy_providers`); err != nil {
			return fmt.Errorf("import original providers: %w", err)
		}
	} else if version == 0 {
		now := time.Now().Unix()
		for _, provider := range DefaultProviders {
			if _, err := tx.ExecContext(ctx, `INSERT INTO acme_providers (name, directory_url, created_at) VALUES (?, ?, ?)`, provider.Name, provider.DirectoryURL, now); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM acme_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO acme_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

// The original application used prefixed tables but had no schema version.
// Its certificate and issuance models differ from the current ones. Upgrade
// provider-only databases automatically, retaining the old tables as backups;
// refuse populated databases rather than silently losing certificate state.
func archiveOriginalSchema(ctx context.Context, tx *sql.Tx) (bool, error) {
	var original bool
	if err := tx.QueryRowContext(ctx, `SELECT
EXISTS(SELECT 1 FROM pragma_table_info('acme_providers') WHERE name = 'terms_of_service') AND
EXISTS(SELECT 1 FROM pragma_table_info('acme_accounts') WHERE name = 'account_url') AND
EXISTS(SELECT 1 FROM pragma_table_info('certificates') WHERE name = 'private_key_pem')`).Scan(&original); err != nil {
		return false, err
	}
	if !original {
		return false, nil
	}
	tables := []string{"acme_providers", "acme_accounts", "acme_orders", "certificates", "acme_authorizations", "acme_challenges", "acme_issuances"}
	for _, table := range tables[1:] {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			return false, fmt.Errorf("inspect original database: %w", err)
		}
		if count > 0 {
			return false, fmt.Errorf("original database contains records in %s; automatic upgrade supports provider-only databases; keep this database and use --db with a new path to start a separate database", table)
		}
	}
	for _, table := range tables {
		backup := "acme_legacy_" + strings.TrimPrefix(table, "acme_")
		if _, err := tx.ExecContext(ctx, `ALTER TABLE `+table+` RENAME TO `+backup); err != nil {
			return false, fmt.Errorf("archive original database: %w", err)
		}
	}
	return true, nil
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
	return list(ctx, s.db, scanProvider, `SELECT `+providerColumns+` FROM acme_providers ORDER BY id`)
}

func (s *SQLite) GetProvider(ctx context.Context, id int64) (*api.Provider, error) {
	p, err := scanProvider(s.db.QueryRowContext(ctx, `SELECT `+providerColumns+` FROM acme_providers WHERE id = ?`, id))
	if err != nil {
		return nil, mapError(err, "provider")
	}
	return &p, nil
}

func (s *SQLite) CreateProvider(ctx context.Context, p *api.Provider) error {
	p.CreatedAt = time.Now().UTC().Truncate(time.Second)
	result, err := s.db.ExecContext(ctx, `INSERT INTO acme_providers (name, directory_url, created_at) VALUES (?, ?, ?)`, p.Name, p.DirectoryURL, unix(p.CreatedAt))
	if err != nil {
		return mapError(err, "provider")
	}
	p.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) DeleteProvider(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM acme_providers WHERE id = ?`, id)
	return checkAffected(result, err, "provider")
}

// Accounts

const accountColumns = `a.id, a.provider_id, p.name, a.email, a.url, a.status, a.thumbprint, a.key_pem, a.created_at, a.updated_at`
const accountFrom = ` FROM acme_accounts a JOIN acme_providers p ON p.id = a.provider_id`

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
	result, err := s.db.ExecContext(ctx, `INSERT INTO acme_accounts (provider_id, email, url, status, thumbprint, key_pem, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ProviderID, a.Email, a.URL, a.Status, a.Thumbprint, a.KeyPEM, unix(now), unix(now))
	if err != nil {
		return mapError(err, "account")
	}
	a.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) UpdateAccountStatus(ctx context.Context, id int64, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE acme_accounts SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), id)
	return checkAffected(result, err, "account")
}

func (s *SQLite) DeleteAccount(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM acme_accounts WHERE id = ?`, id)
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
	return list(ctx, s.db, scanCertificate, `SELECT `+certificateColumns+` FROM acme_certificates ORDER BY id DESC`)
}

func (s *SQLite) GetCertificate(ctx context.Context, id int64) (*api.Certificate, error) {
	c, err := scanCertificate(s.db.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM acme_certificates WHERE id = ?`, id))
	if err != nil {
		return nil, mapError(err, "certificate")
	}
	return &c, nil
}

func (s *SQLite) CreateCertificate(ctx context.Context, c *api.Certificate) error {
	now := time.Now().UTC().Truncate(time.Second)
	c.CreatedAt, c.UpdatedAt = now, now
	result, err := s.db.ExecContext(ctx, `INSERT INTO acme_certificates (domains, account_id, challenge_type, auto_renew, status, cert_pem, key_pem, issuer, serial_number, not_before, not_after, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		joinDomains(c.Domains), c.AccountID, c.ChallengeType, c.AutoRenew, c.Status, c.CertPEM, c.KeyPEM, c.Issuer, c.SerialNumber, unix(c.NotBefore), unix(c.NotAfter), unix(now), unix(now))
	if err != nil {
		return mapError(err, "certificate")
	}
	c.ID, err = result.LastInsertId()
	return err
}

func (s *SQLite) UpdateCertificate(ctx context.Context, c *api.Certificate) error {
	c.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	result, err := s.db.ExecContext(ctx, `UPDATE acme_certificates SET domains = ?, account_id = ?, challenge_type = ?, auto_renew = ?, status = ?, cert_pem = ?, key_pem = ?, issuer = ?, serial_number = ?, not_before = ?, not_after = ?, updated_at = ? WHERE id = ?`,
		joinDomains(c.Domains), c.AccountID, c.ChallengeType, c.AutoRenew, c.Status, c.CertPEM, c.KeyPEM, c.Issuer, c.SerialNumber, unix(c.NotBefore), unix(c.NotAfter), unix(c.UpdatedAt), c.ID)
	return checkAffected(result, err, "certificate")
}

func (s *SQLite) DeleteCertificate(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM acme_certificates WHERE id = ?`, id)
	return checkAffected(result, err, "certificate")
}

// Issuances

const issuanceColumns = `id, certificate_id, account_id, domains, challenge_type, status, step, error, dns_records, csr_pem, key_pem, dns_confirmed, order_url, attempts, next_attempt_at, created_at, updated_at`

func scanIssuance(row scanner) (api.Issuance, error) {
	var i api.Issuance
	var domains, records string
	var created, updated, nextAttempt int64
	if err := row.Scan(&i.ID, &i.CertificateID, &i.AccountID, &domains, &i.ChallengeType, &i.Status, &i.Step, &i.Error, &records, &i.CSRPEM, &i.KeyPEM, &i.DNSConfirmed, &i.OrderURL, &i.Attempts, &nextAttempt, &created, &updated); err != nil {
		return i, err
	}
	i.Domains = splitDomains(domains)
	i.CreatedAt, i.UpdatedAt = fromUnix(created), fromUnix(updated)
	if nextAttempt != 0 {
		i.NextAttemptAt = time.UnixMilli(nextAttempt).UTC()
	}
	return i, json.Unmarshal([]byte(records), &i.DNSRecords)
}

func (s *SQLite) ListIssuances(ctx context.Context, certificateID int64) ([]api.Issuance, error) {
	if certificateID > 0 {
		return list(ctx, s.db, scanIssuance, `SELECT `+issuanceColumns+` FROM acme_issuances WHERE certificate_id = ? ORDER BY id DESC`, certificateID)
	}
	return list(ctx, s.db, scanIssuance, `SELECT `+issuanceColumns+` FROM acme_issuances ORDER BY id DESC`)
}

func (s *SQLite) GetIssuance(ctx context.Context, id int64) (*api.Issuance, error) {
	i, err := scanIssuance(s.db.QueryRowContext(ctx, `SELECT `+issuanceColumns+` FROM acme_issuances WHERE id = ?`, id))
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
	result, err := s.db.ExecContext(ctx, `INSERT INTO acme_issuances (certificate_id, account_id, domains, challenge_type, status, step, error, dns_records, csr_pem, key_pem, dns_confirmed, order_url, attempts, next_attempt_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.CertificateID, i.AccountID, joinDomains(i.Domains), i.ChallengeType, i.Status, i.Step, i.Error, string(records), i.CSRPEM, i.KeyPEM, i.DNSConfirmed, i.OrderURL, i.Attempts, retryMillis(i.NextAttemptAt), unix(now), unix(now))
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
	result, err := s.db.ExecContext(ctx, `UPDATE acme_issuances SET status = ?, step = ?, error = ?, dns_records = ?, csr_pem = ?, key_pem = ?, dns_confirmed = ?, order_url = ?, attempts = ?, next_attempt_at = ?, updated_at = ? WHERE id = ?`,
		i.Status, i.Step, i.Error, string(records), i.CSRPEM, i.KeyPEM, i.DNSConfirmed, i.OrderURL, i.Attempts, retryMillis(i.NextAttemptAt), unix(i.UpdatedAt), i.ID)
	return checkAffected(result, err, "issuance")
}

func retryMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
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
	return list(ctx, s.db, scanOrder, `SELECT `+orderColumns+` FROM acme_orders ORDER BY id DESC`)
}

func (s *SQLite) CreateOrder(ctx context.Context, o *api.Order) error {
	now := time.Now().UTC().Truncate(time.Second)
	o.CreatedAt, o.UpdatedAt = now, now
	var issuanceID any
	if o.IssuanceID > 0 {
		issuanceID = o.IssuanceID
	}
	// A CA may hand back an unfinished order it already gave the account for
	// the same names (Let's Encrypt does after a failed attempt): the retry
	// takes that order over.
	var created int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO acme_orders (account_id, issuance_id, url, domains, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (url) DO UPDATE SET issuance_id = excluded.issuance_id, status = excluded.status, updated_at = excluded.updated_at
		RETURNING id, created_at`,
		o.AccountID, issuanceID, o.URL, joinDomains(o.Domains), o.Status, unix(now), unix(now)).Scan(&o.ID, &created)
	if err != nil {
		return mapError(err, "order")
	}
	o.CreatedAt = time.Unix(created, 0).UTC()
	return nil
}

func (s *SQLite) UpdateOrderStatus(ctx context.Context, id int64, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE acme_orders SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), id)
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
