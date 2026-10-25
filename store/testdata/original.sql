CREATE TABLE certificates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER DEFAULT 0,
			domains TEXT NOT NULL,
			csr_pem TEXT NOT NULL,
			private_key_pem TEXT NOT NULL,
			cert_pem TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
CREATE INDEX idx_certificates_status ON certificates(status);
CREATE INDEX idx_certificates_order_id ON certificates(order_id);
CREATE TABLE acme_providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			directory_url TEXT NOT NULL,
			terms_of_service TEXT,
			website TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
CREATE INDEX idx_acme_providers_name ON acme_providers(name);
CREATE TABLE acme_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			account_url TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'valid',
			key_pem TEXT NOT NULL,
			thumbprint TEXT,
			agreed_terms BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (provider_id) REFERENCES acme_providers(id) ON DELETE CASCADE
		);
CREATE INDEX idx_acme_accounts_provider_id ON acme_accounts(provider_id);
CREATE TABLE acme_orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			order_url TEXT NOT NULL UNIQUE,
			domains TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (account_id) REFERENCES acme_accounts(id) ON DELETE CASCADE
		);
CREATE INDEX idx_acme_orders_account_id ON acme_orders(account_id);
CREATE TABLE acme_authorizations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER NOT NULL,
			authz_url TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			identifier_type TEXT NOT NULL,
			identifier_value TEXT NOT NULL,
			expires DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES acme_orders(id) ON DELETE CASCADE
		);
CREATE INDEX idx_acme_authorizations_order_id ON acme_authorizations(order_id);
CREATE TABLE acme_challenges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			authorization_id INTEGER NOT NULL,
			challenge_url TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			token TEXT NOT NULL,
			key_auth TEXT,
			validated DATETIME,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (authorization_id) REFERENCES acme_authorizations(id) ON DELETE CASCADE
		);
CREATE INDEX idx_acme_challenges_authz_id ON acme_challenges(authorization_id);
CREATE TABLE acme_issuances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			certificate_id INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			order_id INTEGER NOT NULL DEFAULT 0,
			domains TEXT NOT NULL,
			challenge_type TEXT NOT NULL,
			dns_target TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			step TEXT NOT NULL DEFAULT 'queued',
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
CREATE INDEX idx_acme_issuances_certificate ON acme_issuances(certificate_id);
CREATE INDEX idx_acme_issuances_status ON acme_issuances(status);
