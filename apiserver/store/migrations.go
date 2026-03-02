package store

import (
	"database/sql"
	"fmt"
)

// migrations is an ordered list of SQL statements that define the database schema.
// Each migration uses CREATE TABLE IF NOT EXISTS and CREATE INDEX IF NOT EXISTS
// so that RunMigrations is idempotent.
var migrations = []string{
	// Enable the uuid-ossp extension for uuid_generate_v4() if needed,
	// though we generate UUIDs in Go. This is kept for completeness.
	`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,

	`CREATE TABLE IF NOT EXISTS users (
		id            UUID PRIMARY KEY,
		email         TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		display_name  TEXT NOT NULL DEFAULT '',
		created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,

	`CREATE TABLE IF NOT EXISTS tenants (
		id           UUID PRIMARY KEY,
		name         TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL DEFAULT '',
		owner_id     UUID NOT NULL REFERENCES users(id),
		plan         TEXT NOT NULL DEFAULT 'free'
			CHECK (plan IN ('free', 'starter', 'pro', 'enterprise')),
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,

	`CREATE INDEX IF NOT EXISTS idx_tenants_owner_id ON tenants(owner_id)`,

	`CREATE TABLE IF NOT EXISTS tenant_members (
		id         UUID PRIMARY KEY,
		tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role       TEXT NOT NULL DEFAULT 'member'
			CHECK (role IN ('owner', 'admin', 'member')),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (tenant_id, user_id)
	)`,

	`CREATE INDEX IF NOT EXISTS idx_tenant_members_user_id ON tenant_members(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_tenant_members_tenant_id ON tenant_members(tenant_id)`,

	`CREATE TABLE IF NOT EXISTS apps (
		id                 UUID PRIMARY KEY,
		tenant_id          UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		name               TEXT NOT NULL,
		image              TEXT NOT NULL DEFAULT '',
		port               INTEGER NOT NULL DEFAULT 8080,
		min_instances      INTEGER NOT NULL DEFAULT 0,
		max_instances      INTEGER NOT NULL DEFAULT 10,
		target_concurrency INTEGER NOT NULL DEFAULT 100,
		cpu_request        TEXT NOT NULL DEFAULT '100m',
		memory_limit       TEXT NOT NULL DEFAULT '128Mi',
		env_vars           JSONB NOT NULL DEFAULT '{}',
		paused             BOOLEAN NOT NULL DEFAULT false,
		created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (tenant_id, name)
	)`,

	`CREATE INDEX IF NOT EXISTS idx_apps_tenant_id ON apps(tenant_id)`,

	`CREATE TABLE IF NOT EXISTS api_keys (
		id           UUID PRIMARY KEY,
		tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		name         TEXT NOT NULL DEFAULT '',
		key_hash     TEXT NOT NULL UNIQUE,
		key_prefix   TEXT NOT NULL,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		expires_at   TIMESTAMPTZ,
		last_used_at TIMESTAMPTZ
	)`,

	`CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_id ON api_keys(tenant_id)`,
	`CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash)`,
}

// RunMigrations executes all schema migrations against the provided database.
// Migrations are idempotent and safe to run on every application startup.
func RunMigrations(db *sql.DB) error {
	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}
	return nil
}
