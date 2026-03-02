package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
)

// App represents a deployed application stored in the database.
type App struct {
	ID                string            `json:"id"`
	TenantID          string            `json:"tenantId"`
	Name              string            `json:"name"`
	Image             string            `json:"image"`
	Port              int               `json:"port"`
	MinInstances      int               `json:"minInstances"`
	MaxInstances      int               `json:"maxInstances"`
	TargetConcurrency int               `json:"targetConcurrency"`
	CPURequest        string            `json:"cpuRequest"`
	MemoryLimit       string            `json:"memoryLimit"`
	EnvVars           map[string]string `json:"envVars"`
	Paused            bool              `json:"paused"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`

	// K8s CRD status (enriched at read time, not stored in DB).
	Phase          string `json:"phase"`
	URL            string `json:"url"`
	LatestRevision string `json:"latest_revision"`
	ReadyInstances int32  `json:"ready_instances"`
}

// Domain represents a custom domain mapping for an app.
type Domain struct {
	Hostname string `json:"hostname"`
}

// AppStore defines the data access interface for app operations.
type AppStore interface {
	CreateApp(ctx context.Context, app *App) error
	GetAppByID(ctx context.Context, id string) (*App, error)
	ListAppsByTenant(ctx context.Context, tenantID string) ([]*App, error)
	UpdateApp(ctx context.Context, app *App) error
	DeleteApp(ctx context.Context, id string) error
}

// CreateAppRequest holds the input for creating a new app.
type CreateAppRequest struct {
	Name              string            `json:"name"`
	Image             string            `json:"image"`
	Port              int               `json:"port,omitempty"`
	MinInstances      int               `json:"minInstances,omitempty"`
	MaxInstances      int               `json:"maxInstances,omitempty"`
	TargetConcurrency int               `json:"targetConcurrency,omitempty"`
	CPURequest        string            `json:"cpuRequest,omitempty"`
	MemoryLimit       string            `json:"memoryLimit,omitempty"`
	EnvVars           map[string]string `json:"envVars,omitempty"`
}

// UpdateAppRequest holds the input for updating an existing app.
type UpdateAppRequest struct {
	Image             string `json:"image,omitempty"`
	Port              int    `json:"port,omitempty"`
	MinInstances      *int   `json:"minInstances,omitempty"`
	MaxInstances      *int   `json:"maxInstances,omitempty"`
	TargetConcurrency *int   `json:"targetConcurrency,omitempty"`
	CPURequest        string `json:"cpuRequest,omitempty"`
	MemoryLimit       string `json:"memoryLimit,omitempty"`
	Paused            *bool  `json:"paused,omitempty"`
}

// UpdateEnvRequest replaces the entire set of environment variables.
type UpdateEnvRequest struct {
	EnvVars map[string]string `json:"envVars"`
}

// PatchEnvRequest merges the provided environment variables with the existing set.
// Keys mapped to an empty string are deleted.
type PatchEnvRequest struct {
	EnvVars map[string]string `json:"envVars"`
}

// AddDomainRequest holds the input for adding a custom domain.
type AddDomainRequest struct {
	Hostname string `json:"hostname"`
}

// AppService handles app business logic.
type AppService struct {
	store     AppStore
	tenants   TenantStore
	k8sClient K8sClient
}

// NewAppService creates a new AppService.
func NewAppService(store AppStore, tenants TenantStore, k8sClient K8sClient) *AppService {
	return &AppService{
		store:     store,
		tenants:   tenants,
		k8sClient: k8sClient,
	}
}

// Create creates a new app within the specified tenant.
func (s *AppService) Create(ctx context.Context, tenantID string, req CreateAppRequest) (*App, error) {
	if req.Name == "" {
		return nil, errors.New("app name is required")
	}
	if req.Image == "" {
		return nil, errors.New("image is required")
	}

	// Apply defaults.
	if req.Port == 0 {
		req.Port = 8080
	}
	if req.MaxInstances == 0 {
		req.MaxInstances = 10
	}
	if req.TargetConcurrency == 0 {
		req.TargetConcurrency = 100
	}
	if req.CPURequest == "" {
		req.CPURequest = "100m"
	}
	if req.MemoryLimit == "" {
		req.MemoryLimit = "128Mi"
	}
	if req.EnvVars == nil {
		req.EnvVars = make(map[string]string)
	}

	now := time.Now()
	app := &App{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		Name:              req.Name,
		Image:             req.Image,
		Port:              req.Port,
		MinInstances:      req.MinInstances,
		MaxInstances:      req.MaxInstances,
		TargetConcurrency: req.TargetConcurrency,
		CPURequest:        req.CPURequest,
		MemoryLimit:       req.MemoryLimit,
		EnvVars:           req.EnvVars,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.store.CreateApp(ctx, app); err != nil {
		return nil, err
	}

	s.syncAppToK8s(ctx, app)

	return app, nil
}

// List returns all apps for the given tenant.
func (s *AppService) List(ctx context.Context, tenantID string) ([]*App, error) {
	apps, err := s.store.ListAppsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if s.k8sClient != nil && len(apps) > 0 {
		// Resolve tenant name once for the batch.
		tenant, tErr := s.tenants.GetTenantByID(ctx, apps[0].TenantID)
		if tErr == nil {
			for _, app := range apps {
				s.enrichWithK8sStatus(ctx, app, tenant.Name)
			}
		}
	}
	return apps, nil
}

// Get returns a single app by ID.
func (s *AppService) Get(ctx context.Context, appID string) (*App, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}
	if s.k8sClient != nil {
		tenant, tErr := s.tenants.GetTenantByID(ctx, app.TenantID)
		if tErr == nil {
			s.enrichWithK8sStatus(ctx, app, tenant.Name)
		}
	}
	return app, nil
}

// enrichWithK8sStatus fetches the App CRD status from K8s and copies the
// fields into the service.App. Errors are logged but not propagated.
func (s *AppService) enrichWithK8sStatus(ctx context.Context, app *App, tenantName string) {
	status, err := s.k8sClient.GetAppCRDStatus(ctx, app.Name, tenantName)
	if err != nil {
		log.Printf("WARNING: failed to get App CRD status for %q: %v", app.Name, err)
		return
	}
	app.Phase = status.Phase
	app.URL = status.URL
	app.LatestRevision = status.LatestRevision
	app.ReadyInstances = status.ReadyInstances
}

// Update modifies an existing app.
func (s *AppService) Update(ctx context.Context, appID string, req UpdateAppRequest) (*App, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	if req.Image != "" {
		app.Image = req.Image
	}
	if req.Port != 0 {
		app.Port = req.Port
	}
	if req.MinInstances != nil {
		app.MinInstances = *req.MinInstances
	}
	if req.MaxInstances != nil {
		app.MaxInstances = *req.MaxInstances
	}
	if req.TargetConcurrency != nil {
		app.TargetConcurrency = *req.TargetConcurrency
	}
	if req.CPURequest != "" {
		app.CPURequest = req.CPURequest
	}
	if req.MemoryLimit != "" {
		app.MemoryLimit = req.MemoryLimit
	}
	if req.Paused != nil {
		app.Paused = *req.Paused
	}
	app.UpdatedAt = time.Now()

	if err := s.store.UpdateApp(ctx, app); err != nil {
		return nil, err
	}

	s.syncAppToK8s(ctx, app)

	return app, nil
}

// Delete removes an app from the database and Kubernetes.
func (s *AppService) Delete(ctx context.Context, appID string) error {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return err
	}

	if err := s.store.DeleteApp(ctx, appID); err != nil {
		return err
	}

	if s.k8sClient != nil {
		tenant, tErr := s.tenants.GetTenantByID(ctx, app.TenantID)
		if tErr == nil {
			_ = s.k8sClient.DeleteAppCRD(ctx, app.Name, tenant.Name)
		}
	}

	return nil
}

// Redeploy triggers a redeployment of the app by bumping the updated_at
// timestamp and syncing to Kubernetes.
func (s *AppService) Redeploy(ctx context.Context, appID string) (*App, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	app.UpdatedAt = time.Now()
	if err := s.store.UpdateApp(ctx, app); err != nil {
		return nil, err
	}

	s.syncAppToK8s(ctx, app)

	return app, nil
}

// GetEnv returns the environment variables for the specified app.
func (s *AppService) GetEnv(ctx context.Context, appID string) (map[string]string, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}
	return app.EnvVars, nil
}

// UpdateEnv replaces all environment variables on the app.
func (s *AppService) UpdateEnv(ctx context.Context, appID string, envVars map[string]string) (map[string]string, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	app.EnvVars = envVars
	app.UpdatedAt = time.Now()

	if err := s.store.UpdateApp(ctx, app); err != nil {
		return nil, err
	}

	s.syncAppToK8s(ctx, app)

	return app.EnvVars, nil
}

// PatchEnv merges the provided environment variables into the existing set.
// Keys with empty-string values are deleted.
func (s *AppService) PatchEnv(ctx context.Context, appID string, patch map[string]string) (map[string]string, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	if app.EnvVars == nil {
		app.EnvVars = make(map[string]string)
	}
	for k, v := range patch {
		if v == "" {
			delete(app.EnvVars, k)
		} else {
			app.EnvVars[k] = v
		}
	}
	app.UpdatedAt = time.Now()

	if err := s.store.UpdateApp(ctx, app); err != nil {
		return nil, err
	}

	s.syncAppToK8s(ctx, app)

	return app.EnvVars, nil
}

// ListDomains returns the custom domains for an app. Domains are stored as
// a JSON list in the EnvVars map under the reserved key "_domains".
func (s *AppService) ListDomains(ctx context.Context, appID string) ([]Domain, error) {
	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	domains := parseDomains(app.EnvVars["_domains"])
	if domains == nil {
		domains = []Domain{}
	}
	return domains, nil
}

// AddDomain adds a custom domain to the app.
func (s *AppService) AddDomain(ctx context.Context, appID string, hostname string) ([]Domain, error) {
	if hostname == "" {
		return nil, errors.New("hostname is required")
	}

	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	if app.EnvVars == nil {
		app.EnvVars = make(map[string]string)
	}

	domains := parseDomains(app.EnvVars["_domains"])
	for _, d := range domains {
		if d.Hostname == hostname {
			return nil, errors.New("domain already exists")
		}
	}
	domains = append(domains, Domain{Hostname: hostname})
	app.EnvVars["_domains"] = encodeDomains(domains)
	app.UpdatedAt = time.Now()

	if err := s.store.UpdateApp(ctx, app); err != nil {
		return nil, err
	}

	s.syncAppToK8s(ctx, app)

	return domains, nil
}

// RemoveDomain removes a custom domain from the app.
func (s *AppService) RemoveDomain(ctx context.Context, appID string, hostname string) error {
	if hostname == "" {
		return errors.New("hostname is required")
	}

	app, err := s.store.GetAppByID(ctx, appID)
	if err != nil {
		return err
	}

	domains := parseDomains(app.EnvVars["_domains"])
	filtered := make([]Domain, 0, len(domains))
	found := false
	for _, d := range domains {
		if d.Hostname == hostname {
			found = true
			continue
		}
		filtered = append(filtered, d)
	}
	if !found {
		return errors.New("domain not found")
	}

	if app.EnvVars == nil {
		app.EnvVars = make(map[string]string)
	}
	app.EnvVars["_domains"] = encodeDomains(filtered)
	app.UpdatedAt = time.Now()

	if err := s.store.UpdateApp(ctx, app); err != nil {
		return err
	}

	s.syncAppToK8s(ctx, app)

	return nil
}

// syncAppToK8s synchronises the app state to Kubernetes. Errors are logged but
// not propagated to the caller.
func (s *AppService) syncAppToK8s(ctx context.Context, app *App) {
	if s.k8sClient == nil {
		return
	}
	tenant, err := s.tenants.GetTenantByID(ctx, app.TenantID)
	if err != nil {
		log.Printf("WARNING: failed to look up tenant %q for app CRD sync: %v", app.TenantID, err)
		return
	}
	if err := s.k8sClient.UpdateAppCRD(ctx, app, tenant.Name); err != nil {
		log.Printf("WARNING: failed to sync App CRD %q to K8s: %v", app.Name, err)
	}
}

// parseDomains parses a comma-separated list of hostnames into Domain objects.
func parseDomains(raw string) []Domain {
	if raw == "" {
		return nil
	}
	var domains []Domain
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			h := raw[start:i]
			if h != "" {
				domains = append(domains, Domain{Hostname: h})
			}
			start = i + 1
		}
	}
	return domains
}

// encodeDomains serialises a list of domains into a comma-separated string.
func encodeDomains(domains []Domain) string {
	if len(domains) == 0 {
		return ""
	}
	result := ""
	for i, d := range domains {
		if i > 0 {
			result += ","
		}
		result += d.Hostname
	}
	return result
}
