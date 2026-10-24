
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
