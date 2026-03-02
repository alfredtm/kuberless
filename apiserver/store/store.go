package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// User represents a platform user account.
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Tenant represents an organization or workspace that owns apps.
type Tenant struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	OwnerID     uuid.UUID `json:"owner_id"`
	Plan        string    `json:"plan"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TenantMember represents a user's membership and role within a tenant.
type TenantMember struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// EnvVars is a map of environment variable key-value pairs stored as JSONB.
type EnvVars map[string]string

// Scan implements the sql.Scanner interface for reading JSONB from PostgreSQL.
func (e *EnvVars) Scan(src interface{}) error {
	if src == nil {
		*e = make(EnvVars)
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("envvars: unsupported scan source type %T", src)
	}
	return json.Unmarshal(data, e)
}

// Value implements the driver.Valuer interface for writing JSONB to PostgreSQL.
func (e EnvVars) Value() ([]byte, error) {
	if e == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(e)
}

// App represents a deployable application within a tenant.
type App struct {
	ID                uuid.UUID `json:"id"`
	TenantID          uuid.UUID `json:"tenant_id"`
	Name              string    `json:"name"`
	Image             string    `json:"image"`
	Port              int       `json:"port"`
	MinInstances      int       `json:"min_instances"`
	MaxInstances      int       `json:"max_instances"`
	TargetConcurrency int       `json:"target_concurrency"`
	CPURequest        string    `json:"cpu_request"`
	MemoryLimit       string    `json:"memory_limit"`
	EnvVars           EnvVars   `json:"env_vars"`
	Paused            bool      `json:"paused"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// APIKey represents an API key used to authenticate requests for a tenant.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Store provides data access methods backed by a PostgreSQL database.
type Store struct {
	db *sql.DB
}

// New opens a PostgreSQL connection using the provided DSN and returns an
// initialised Store. The caller is responsible for calling Close when the
// store is no longer needed.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping db: %w", err)
	}
	return &Store{db: db}, nil
}

// NewFromDB wraps an existing *sql.DB connection as a Store.
func NewFromDB(db *sql.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying *sql.DB, useful for running migrations.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close releases the database connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}
