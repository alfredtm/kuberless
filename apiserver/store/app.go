package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateApp inserts a new application for the given tenant.
func (s *Store) CreateApp(ctx context.Context, app *App) (*App, error) {
	app.ID = uuid.New()
	now := time.Now().UTC()
	app.CreatedAt = now
	app.UpdatedAt = now

	if app.EnvVars == nil {
		app.EnvVars = make(EnvVars)
	}

	envJSON, err := json.Marshal(app.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("store: marshal env vars: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO apps (id, tenant_id, name, image, port, min_instances, max_instances,
		                    target_concurrency, cpu_request, memory_limit, env_vars, paused,
		                    created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		app.ID, app.TenantID, app.Name, app.Image, app.Port,
		app.MinInstances, app.MaxInstances, app.TargetConcurrency,
		app.CPURequest, app.MemoryLimit, envJSON, app.Paused,
		app.CreatedAt, app.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: create app: %w", err)
	}
	return app, nil
}

// GetAppByID retrieves an application by its unique identifier.
func (s *Store) GetAppByID(ctx context.Context, id uuid.UUID) (*App, error) {
	a := &App{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, image, port, min_instances, max_instances,
		        target_concurrency, cpu_request, memory_limit, env_vars, paused,
		        created_at, updated_at
		 FROM apps WHERE id = $1`, id,
	).Scan(
		&a.ID, &a.TenantID, &a.Name, &a.Image, &a.Port,
		&a.MinInstances, &a.MaxInstances, &a.TargetConcurrency,
		&a.CPURequest, &a.MemoryLimit, &a.EnvVars, &a.Paused,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: app not found: %w", err)
		}
		return nil, fmt.Errorf("store: get app by id: %w", err)
	}
	return a, nil
}

// GetAppByName retrieves an application by name within a specific tenant.
func (s *Store) GetAppByName(ctx context.Context, tenantID uuid.UUID, name string) (*App, error) {
	a := &App{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, image, port, min_instances, max_instances,
		        target_concurrency, cpu_request, memory_limit, env_vars, paused,
		        created_at, updated_at
		 FROM apps WHERE tenant_id = $1 AND name = $2`, tenantID, name,
	).Scan(
		&a.ID, &a.TenantID, &a.Name, &a.Image, &a.Port,
		&a.MinInstances, &a.MaxInstances, &a.TargetConcurrency,
		&a.CPURequest, &a.MemoryLimit, &a.EnvVars, &a.Paused,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: app not found: %w", err)
		}
		return nil, fmt.Errorf("store: get app by name: %w", err)
	}
	return a, nil
}

// ListAppsByTenant returns all applications belonging to the given tenant.
func (s *Store) ListAppsByTenant(ctx context.Context, tenantID uuid.UUID) ([]App, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, image, port, min_instances, max_instances,
		        target_concurrency, cpu_request, memory_limit, env_vars, paused,
		        created_at, updated_at
		 FROM apps WHERE tenant_id = $1
		 ORDER BY name ASC`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list apps by tenant: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var apps []App
	for rows.Next() {
		var a App
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.Name, &a.Image, &a.Port,
			&a.MinInstances, &a.MaxInstances, &a.TargetConcurrency,
			&a.CPURequest, &a.MemoryLimit, &a.EnvVars, &a.Paused,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan app: %w", err)
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate apps: %w", err)
	}
	return apps, nil
}

// UpdateApp updates the mutable fields of an application. The app ID must be set.
func (s *Store) UpdateApp(ctx context.Context, app *App) (*App, error) {
	now := time.Now().UTC()

	if app.EnvVars == nil {
		app.EnvVars = make(EnvVars)
	}

	envJSON, err := json.Marshal(app.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("store: marshal env vars: %w", err)
	}

	updated := &App{}
	err = s.db.QueryRowContext(ctx,
		`UPDATE apps
		 SET image = $2, port = $3, min_instances = $4, max_instances = $5,
		     target_concurrency = $6, cpu_request = $7, memory_limit = $8,
		     env_vars = $9, paused = $10, updated_at = $11
		 WHERE id = $1
		 RETURNING id, tenant_id, name, image, port, min_instances, max_instances,
		           target_concurrency, cpu_request, memory_limit, env_vars, paused,
		           created_at, updated_at`,
		app.ID, app.Image, app.Port, app.MinInstances, app.MaxInstances,
		app.TargetConcurrency, app.CPURequest, app.MemoryLimit,
		envJSON, app.Paused, now,
	).Scan(
		&updated.ID, &updated.TenantID, &updated.Name, &updated.Image, &updated.Port,
		&updated.MinInstances, &updated.MaxInstances, &updated.TargetConcurrency,
		&updated.CPURequest, &updated.MemoryLimit, &updated.EnvVars, &updated.Paused,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("store: app not found: %w", err)
		}
		return nil, fmt.Errorf("store: update app: %w", err)
	}
	return updated, nil
}

// DeleteApp removes an application by its unique identifier.
func (s *Store) DeleteApp(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM apps WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete app: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete app rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("store: app not found: %w", sql.ErrNoRows)
	}
	return nil
}
