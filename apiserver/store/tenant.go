package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateTenant inserts a new tenant and adds the owner as a tenant member with
// the "owner" role. Both operations are performed within a transaction.
func (s *Store) CreateTenant(ctx context.Context, name, displayName string, ownerID uuid.UUID, plan string) (*Tenant, error) {
	if plan == "" {
		plan = "free"
	}

	t := &Tenant{
		ID:          uuid.New(),
		Name:        name,
		DisplayName: displayName,
		OwnerID:     ownerID,
		Plan:        plan,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO tenants (id, name, display_name, owner_id, plan, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.ID, t.Name, t.DisplayName, t.OwnerID, t.Plan, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: create tenant: %w", err)
	}

	// Automatically add the owner as a tenant member with the "owner" role.
	memberID := uuid.New()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO tenant_members (id, tenant_id, user_id, role, created_at)
		 VALUES ($1, $2, $3, 'owner', $4)`,
		memberID, t.ID, ownerID, t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: add owner as tenant member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit tx: %w", err)
	}

	return t, nil
}

// GetTenantByID retrieves a tenant by its unique identifier.
func (s *Store) GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	t := &Tenant{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, owner_id, plan, created_at, updated_at
		 FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.DisplayName, &t.OwnerID, &t.Plan, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: tenant not found: %w", err)
		}
		return nil, fmt.Errorf("store: get tenant by id: %w", err)
	}
	return t, nil
}

// GetTenantByName retrieves a tenant by its unique slug name.
func (s *Store) GetTenantByName(ctx context.Context, name string) (*Tenant, error) {
	t := &Tenant{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, owner_id, plan, created_at, updated_at
		 FROM tenants WHERE name = $1`, name,
	).Scan(&t.ID, &t.Name, &t.DisplayName, &t.OwnerID, &t.Plan, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: tenant not found: %w", err)
		}
		return nil, fmt.Errorf("store: get tenant by name: %w", err)
	}
	return t, nil
}

// ListTenantsByUser returns all tenants that the given user is a member of.
func (s *Store) ListTenantsByUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.display_name, t.owner_id, t.plan, t.created_at, t.updated_at
		 FROM tenants t
		 INNER JOIN tenant_members tm ON tm.tenant_id = t.id
		 WHERE tm.user_id = $1
		 ORDER BY t.name ASC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list tenants by user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tenants []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.DisplayName, &t.OwnerID, &t.Plan, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tenants: %w", err)
	}
	return tenants, nil
}

// UpdateTenant updates the mutable fields of a tenant (display_name and plan).
func (s *Store) UpdateTenant(ctx context.Context, id uuid.UUID, displayName, plan string) (*Tenant, error) {
	now := time.Now().UTC()
	t := &Tenant{}
	err := s.db.QueryRowContext(ctx,
		`UPDATE tenants
		 SET display_name = $2, plan = $3, updated_at = $4
		 WHERE id = $1
		 RETURNING id, name, display_name, owner_id, plan, created_at, updated_at`,
		id, displayName, plan, now,
	).Scan(&t.ID, &t.Name, &t.DisplayName, &t.OwnerID, &t.Plan, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: tenant not found: %w", err)
		}
		return nil, fmt.Errorf("store: update tenant: %w", err)
	}
	return t, nil
}

// DeleteTenant removes a tenant and all associated data (apps, members, api_keys)
// via cascading deletes defined in the schema.
func (s *Store) DeleteTenant(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete tenant: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete tenant rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("store: tenant not found: %w", sql.ErrNoRows)
	}
	return nil
}
