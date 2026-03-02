package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// HashAPIKey computes the SHA-256 hash of a raw API key string and returns
// the hex-encoded digest. This is used both when creating a key and when
// looking one up by its plaintext value.
func HashAPIKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}

// CreateAPIKey stores a new API key record. The caller provides the raw key;
// this method hashes it before storage and extracts the first 8 characters as
// the key_prefix for display purposes. The raw key is never persisted.
func (s *Store) CreateAPIKey(ctx context.Context, tenantID uuid.UUID, name, rawKey string, expiresAt *time.Time) (*APIKey, error) {
	if len(rawKey) < 8 {
		return nil, fmt.Errorf("store: api key must be at least 8 characters")
	}

	k := &APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      name,
		KeyHash:   HashAPIKey(rawKey),
		KeyPrefix: rawKey[:8],
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, key_prefix, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		k.ID, k.TenantID, k.Name, k.KeyHash, k.KeyPrefix, k.CreatedAt, k.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: create api key: %w", err)
	}
	return k, nil
}

// GetAPIKeyByHash retrieves an API key record by the SHA-256 hash of the raw
// key. This is the primary lookup path for authenticating incoming requests.
func (s *Store) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	k := &APIKey{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, key_hash, key_prefix, created_at, expires_at, last_used_at
		 FROM api_keys WHERE key_hash = $1`, keyHash,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: api key not found: %w", err)
		}
		return nil, fmt.Errorf("store: get api key by hash: %w", err)
	}
	return k, nil
}

// ListAPIKeysByTenant returns all API keys belonging to the given tenant.
// Key hashes are included in the returned structs but are omitted from JSON
// serialisation by the json:"-" tag on the KeyHash field.
func (s *Store) ListAPIKeysByTenant(ctx context.Context, tenantID uuid.UUID) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, key_hash, key_prefix, created_at, expires_at, last_used_at
		 FROM api_keys WHERE tenant_id = $1
		 ORDER BY created_at DESC`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list api keys by tenant: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt); err != nil {
			return nil, fmt.Errorf("store: scan api key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate api keys: %w", err)
	}
	return keys, nil
}

// DeleteAPIKey removes an API key by its unique identifier.
func (s *Store) DeleteAPIKey(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete api key: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete api key rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("store: api key not found: %w", sql.ErrNoRows)
	}
	return nil
}

// UpdateAPIKeyLastUsed sets the last_used_at timestamp to the current time.
// This is typically called after a successful authentication using the key.
func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = $2 WHERE id = $1`,
		id, now,
	)
	if err != nil {
		return fmt.Errorf("store: update api key last used: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update api key last used rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("store: api key not found: %w", sql.ErrNoRows)
	}
	return nil
}
