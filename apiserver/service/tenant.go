package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

// Tenant represents a platform tenant stored in the database.
type Tenant struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	OwnerID     string    `json:"ownerId"`
	Plan        string    `json:"plan"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TenantMember represents a membership record linking a user to a tenant.
type TenantMember struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// TenantStore defines the data access interface for tenant operations.
type TenantStore interface {
	CreateTenant(ctx context.Context, tenant *Tenant) error
	GetTenantByID(ctx context.Context, id string) (*Tenant, error)
	ListTenantsByUser(ctx context.Context, userID string) ([]*Tenant, error)
	UpdateTenant(ctx context.Context, tenant *Tenant) error
	DeleteTenant(ctx context.Context, id string) error
	AddTenantMember(ctx context.Context, member *TenantMember) error
	UserHasAccessToTenant(ctx context.Context, userID, tenantID string) (bool, error)
}

// AppCRDStatus holds status fields read from the App custom resource in K8s.
type AppCRDStatus struct {
	Phase          string
	URL            string
	LatestRevision string
	ReadyInstances int32
}

// K8sClient defines the interface for Kubernetes operations used by the API server.
type K8sClient interface {
	CreateTenantCRD(ctx context.Context, tenant *Tenant) error
	UpdateTenantCRD(ctx context.Context, tenant *Tenant) error
	DeleteTenantCRD(ctx context.Context, tenantName string) error
	CreateAppCRD(ctx context.Context, app *App, tenantName string) error
	UpdateAppCRD(ctx context.Context, app *App, tenantName string) error
	DeleteAppCRD(ctx context.Context, appName, tenantName string) error
	GetAppCRDStatus(ctx context.Context, appName, tenantName string) (*AppCRDStatus, error)
}

// CreateTenantRequest holds the input for creating a new tenant.
type CreateTenantRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Plan        string `json:"plan"`
}

// UpdateTenantRequest holds the input for updating an existing tenant.
type UpdateTenantRequest struct {
	DisplayName string `json:"displayName,omitempty"`
	Plan        string `json:"plan,omitempty"`
}

// TenantService handles tenant business logic.
type TenantService struct {
	store     TenantStore
	k8sClient K8sClient
}

// NewTenantService creates a new TenantService.
func NewTenantService(store TenantStore, k8sClient K8sClient) *TenantService {
	return &TenantService{
		store:     store,
		k8sClient: k8sClient,
	}
}

// Create creates a new tenant, saves it to the database, creates a Kubernetes
// CRD, and adds the creator as the owner member.
func (s *TenantService) Create(ctx context.Context, userID string, req CreateTenantRequest) (*Tenant, error) {
	if req.Name == "" {
		return nil, errors.New("tenant name is required")
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Name
	}
	if req.Plan == "" {
		req.Plan = "free"
	}
	if !isValidPlan(req.Plan) {
		return nil, errors.New("invalid plan: must be one of free, starter, pro, enterprise")
	}

	now := time.Now()
	tenant := &Tenant{
		ID:          uuid.New().String(),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		OwnerID:     userID,
		Plan:        req.Plan,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateTenant(ctx, tenant); err != nil {
		return nil, err
	}

	// Add the creator as the owner member.
	member := &TenantMember{
		ID:        uuid.New().String(),
		TenantID:  tenant.ID,
		UserID:    userID,
		Role:      "owner",
		CreatedAt: now,
	}
	if err := s.store.AddTenantMember(ctx, member); err != nil {
		return nil, err
	}

	// Create the Tenant CRD in Kubernetes.
	if s.k8sClient != nil {
		if err := s.k8sClient.CreateTenantCRD(ctx, tenant); err != nil {
			log.Printf("WARNING: failed to create Tenant CRD %q: %v", tenant.Name, err)
		}
	}

	return tenant, nil
}

// List returns all tenants the specified user has access to.
func (s *TenantService) List(ctx context.Context, userID string) ([]*Tenant, error) {
	return s.store.ListTenantsByUser(ctx, userID)
}

// Get returns a single tenant by ID.
func (s *TenantService) Get(ctx context.Context, tenantID string) (*Tenant, error) {
	return s.store.GetTenantByID(ctx, tenantID)
}

// Update modifies an existing tenant and synchronises the change to Kubernetes.
func (s *TenantService) Update(ctx context.Context, tenantID string, req UpdateTenantRequest) (*Tenant, error) {
	tenant, err := s.store.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if req.DisplayName != "" {
		tenant.DisplayName = req.DisplayName
	}
	if req.Plan != "" {
		if !isValidPlan(req.Plan) {
			return nil, errors.New("invalid plan: must be one of free, starter, pro, enterprise")
		}
		tenant.Plan = req.Plan
	}
	tenant.UpdatedAt = time.Now()

	if err := s.store.UpdateTenant(ctx, tenant); err != nil {
		return nil, err
	}

	if s.k8sClient != nil {
		_ = s.k8sClient.UpdateTenantCRD(ctx, tenant)
	}

	return tenant, nil
}

// Delete removes a tenant from the database and Kubernetes.
func (s *TenantService) Delete(ctx context.Context, tenantID string) error {
	tenant, err := s.store.GetTenantByID(ctx, tenantID)
	if err != nil {
		return err
	}

	if err := s.store.DeleteTenant(ctx, tenantID); err != nil {
		return err
	}

	if s.k8sClient != nil {
		_ = s.k8sClient.DeleteTenantCRD(ctx, tenant.Name)
	}

	return nil
}

// UserHasAccessToTenant checks whether the given user is a member of the tenant.
// It implements the middleware.TenantAccessChecker interface.
func (s *TenantService) UserHasAccessToTenant(ctx context.Context, userID, tenantID string) (bool, error) {
	return s.store.UserHasAccessToTenant(ctx, userID, tenantID)
}

func isValidPlan(plan string) bool {
	switch plan {
	case "free", "starter", "pro", "enterprise":
		return true
	}
	return false
}
