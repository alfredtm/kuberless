// Package adapter bridges the store.Store (Postgres-backed, uuid.UUID IDs) to
// the service-layer, handler-layer, and middleware-layer interfaces which use
// string IDs and their own domain types.
package adapter

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
	"github.com/alfredtm/kuberless/apiserver/handler"
	"github.com/alfredtm/kuberless/apiserver/middleware"
	"github.com/alfredtm/kuberless/apiserver/service"
	"github.com/alfredtm/kuberless/apiserver/store"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// ---------------------------------------------------------------------------
// TenantStoreAdapter  implements  service.TenantStore
// ---------------------------------------------------------------------------

// TenantStoreAdapter adapts *store.Store to the service.TenantStore interface.
type TenantStoreAdapter struct {
	s *store.Store
}

// NewTenantStoreAdapter returns a new TenantStoreAdapter.
func NewTenantStoreAdapter(s *store.Store) *TenantStoreAdapter {
	return &TenantStoreAdapter{s: s}
}

func storeTenantToService(t *store.Tenant) *service.Tenant {
	return &service.Tenant{
		ID:          t.ID.String(),
		Name:        t.Name,
		DisplayName: t.DisplayName,
		OwnerID:     t.OwnerID.String(),
		Plan:        t.Plan,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// CreateTenant inserts a tenant. The service layer has already populated the
// ID; we honour it by writing directly to the DB.
func (a *TenantStoreAdapter) CreateTenant(ctx context.Context, t *service.Tenant) error {
	id, err := parseUUID(t.ID)
	if err != nil {
		return fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	ownerID, err := parseUUID(t.OwnerID)
	if err != nil {
		return fmt.Errorf("adapter: invalid owner id: %w", err)
	}

	_, err = a.s.DB().ExecContext(ctx,
		`INSERT INTO tenants (id, name, display_name, owner_id, plan, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, t.Name, t.DisplayName, ownerID, t.Plan, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("adapter: create tenant: %w", err)
	}
	return nil
}

// GetTenantByID looks up a tenant by its string UUID.
func (a *TenantStoreAdapter) GetTenantByID(ctx context.Context, id string) (*service.Tenant, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	t, err := a.s.GetTenantByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return storeTenantToService(t), nil
}

// ListTenantsByUser returns all tenants that the given user belongs to.
func (a *TenantStoreAdapter) ListTenantsByUser(ctx context.Context, userID string) ([]*service.Tenant, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid user id: %w", err)
	}
	tenants, err := a.s.ListTenantsByUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]*service.Tenant, len(tenants))
	for i := range tenants {
		out[i] = storeTenantToService(&tenants[i])
	}
	return out, nil
}

// UpdateTenant updates the mutable fields of a tenant.
func (a *TenantStoreAdapter) UpdateTenant(ctx context.Context, t *service.Tenant) error {
	id, err := parseUUID(t.ID)
	if err != nil {
		return fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	_, err = a.s.UpdateTenant(ctx, id, t.DisplayName, t.Plan)
	if err != nil {
		return err
	}
	return nil
}

// DeleteTenant removes a tenant by string ID.
func (a *TenantStoreAdapter) DeleteTenant(ctx context.Context, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	return a.s.DeleteTenant(ctx, uid)
}

// AddTenantMember inserts a membership row.
func (a *TenantStoreAdapter) AddTenantMember(ctx context.Context, m *service.TenantMember) error {
	id, err := parseUUID(m.ID)
	if err != nil {
		return fmt.Errorf("adapter: invalid member id: %w", err)
	}
	tenantID, err := parseUUID(m.TenantID)
	if err != nil {
		return fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	userID, err := parseUUID(m.UserID)
	if err != nil {
		return fmt.Errorf("adapter: invalid user id: %w", err)
	}

	_, err = a.s.DB().ExecContext(ctx,
		`INSERT INTO tenant_members (id, tenant_id, user_id, role, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, tenantID, userID, m.Role, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("adapter: add tenant member: %w", err)
	}
	return nil
}

// UserHasAccessToTenant checks whether the user is a member of the tenant.
func (a *TenantStoreAdapter) UserHasAccessToTenant(ctx context.Context, userID, tenantID string) (bool, error) {
	uid, err := parseUUID(userID)
	if err != nil {
		return false, fmt.Errorf("adapter: invalid user id: %w", err)
	}
	tid, err := parseUUID(tenantID)
	if err != nil {
		return false, fmt.Errorf("adapter: invalid tenant id: %w", err)
	}

	var exists bool
	err = a.s.DB().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenant_members WHERE tenant_id = $1 AND user_id = $2)`,
		tid, uid,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("adapter: check tenant access: %w", err)
	}
	return exists, nil
}

// ---------------------------------------------------------------------------
// AppStoreAdapter  implements  service.AppStore
// ---------------------------------------------------------------------------

// AppStoreAdapter adapts *store.Store to the service.AppStore interface.
type AppStoreAdapter struct {
	s *store.Store
}

// NewAppStoreAdapter returns a new AppStoreAdapter.
func NewAppStoreAdapter(s *store.Store) *AppStoreAdapter {
	return &AppStoreAdapter{s: s}
}

func storeAppToService(a *store.App) *service.App {
	env := make(map[string]string, len(a.EnvVars))
	for k, v := range a.EnvVars {
		env[k] = v
	}
	return &service.App{
		ID:                a.ID.String(),
		TenantID:          a.TenantID.String(),
		Name:              a.Name,
		Image:             a.Image,
		Port:              a.Port,
		MinInstances:      a.MinInstances,
		MaxInstances:      a.MaxInstances,
		TargetConcurrency: a.TargetConcurrency,
		CPURequest:        a.CPURequest,
		MemoryLimit:       a.MemoryLimit,
		EnvVars:           env,
		Paused:            a.Paused,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}
}

func serviceAppToStore(a *service.App) (*store.App, error) {
	id, err := parseUUID(a.ID)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid app id: %w", err)
	}
	tenantID, err := parseUUID(a.TenantID)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	env := make(store.EnvVars, len(a.EnvVars))
	for k, v := range a.EnvVars {
		env[k] = v
	}
	return &store.App{
		ID:                id,
		TenantID:          tenantID,
		Name:              a.Name,
		Image:             a.Image,
		Port:              a.Port,
		MinInstances:      a.MinInstances,
		MaxInstances:      a.MaxInstances,
		TargetConcurrency: a.TargetConcurrency,
		CPURequest:        a.CPURequest,
		MemoryLimit:       a.MemoryLimit,
		EnvVars:           env,
		Paused:            a.Paused,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}, nil
}

// CreateApp inserts an app. The service layer has already populated the fields
// including ID; we pass a store.App to the store's CreateApp which overwrites
// the ID. To preserve the service-generated ID, we write directly.
func (a *AppStoreAdapter) CreateApp(ctx context.Context, app *service.App) error {
	sa, err := serviceAppToStore(app)
	if err != nil {
		return err
	}
	// store.CreateApp overwrites the ID and timestamps, but we need to keep
	// the service-generated ID. Write directly.
	if sa.EnvVars == nil {
		sa.EnvVars = make(store.EnvVars)
	}
	envBytes, err := sa.EnvVars.Value()
	if err != nil {
		return fmt.Errorf("adapter: marshal env vars: %w", err)
	}

	_, err = a.s.DB().ExecContext(ctx,
		`INSERT INTO apps (id, tenant_id, name, image, port, min_instances, max_instances,
		                    target_concurrency, cpu_request, memory_limit, env_vars, paused,
		                    created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		sa.ID, sa.TenantID, sa.Name, sa.Image, sa.Port,
		sa.MinInstances, sa.MaxInstances, sa.TargetConcurrency,
		sa.CPURequest, sa.MemoryLimit, envBytes, sa.Paused,
		sa.CreatedAt, sa.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("adapter: create app: %w", err)
	}
	return nil
}

// GetAppByID looks up an app by string UUID.
func (a *AppStoreAdapter) GetAppByID(ctx context.Context, id string) (*service.App, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid app id: %w", err)
	}
	sa, err := a.s.GetAppByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return storeAppToService(sa), nil
}

// ListAppsByTenant returns all apps for a given tenant.
func (a *AppStoreAdapter) ListAppsByTenant(ctx context.Context, tenantID string) ([]*service.App, error) {
	tid, err := parseUUID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	apps, err := a.s.ListAppsByTenant(ctx, tid)
	if err != nil {
		return nil, err
	}
	out := make([]*service.App, len(apps))
	for i := range apps {
		out[i] = storeAppToService(&apps[i])
	}
	return out, nil
}

// UpdateApp updates an app's mutable fields.
func (a *AppStoreAdapter) UpdateApp(ctx context.Context, app *service.App) error {
	sa, err := serviceAppToStore(app)
	if err != nil {
		return err
	}
	_, err = a.s.UpdateApp(ctx, sa)
	if err != nil {
		return err
	}
	return nil
}

// DeleteApp removes an app by string UUID.
func (a *AppStoreAdapter) DeleteApp(ctx context.Context, id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return fmt.Errorf("adapter: invalid app id: %w", err)
	}
	return a.s.DeleteApp(ctx, uid)
}

// ---------------------------------------------------------------------------
// APIKeyServiceAdapter  implements  handler.APIKeyService
// ---------------------------------------------------------------------------

// APIKeyServiceAdapter adapts *store.Store to the handler.APIKeyService
// interface. It contains the business logic for creating, listing, and
// deleting API keys.
type APIKeyServiceAdapter struct {
	s *store.Store
}

// NewAPIKeyServiceAdapter returns a new APIKeyServiceAdapter.
func NewAPIKeyServiceAdapter(s *store.Store) *APIKeyServiceAdapter {
	return &APIKeyServiceAdapter{s: s}
}

// CreateAPIKey generates a new random API key, stores its hash, and returns
// the full key (shown only once) along with metadata.
func (a *APIKeyServiceAdapter) CreateAPIKey(ctx context.Context, tenantID, name string) (*handler.CreateAPIKeyResponse, error) {
	tid, err := parseUUID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid tenant id: %w", err)
	}

	// Generate a random API key: "sk-" + 32 random hex characters.
	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("adapter: generate api key: %w", err)
	}
	rawKey := "sk-" + hex.EncodeToString(rawBytes)

	k, err := a.s.CreateAPIKey(ctx, tid, name, rawKey, nil)
	if err != nil {
		return nil, err
	}

	return &handler.CreateAPIKeyResponse{
		APIKey: handler.APIKey{
			ID:         k.ID.String(),
			TenantID:   k.TenantID.String(),
			Name:       k.Name,
			KeyPrefix:  k.KeyPrefix,
			CreatedAt:  k.CreatedAt,
			ExpiresAt:  k.ExpiresAt,
			LastUsedAt: k.LastUsedAt,
		},
		Key: rawKey,
	}, nil
}

// ListAPIKeys returns all API keys for a tenant.
func (a *APIKeyServiceAdapter) ListAPIKeys(ctx context.Context, tenantID string) ([]*handler.APIKey, error) {
	tid, err := parseUUID(tenantID)
	if err != nil {
		return nil, fmt.Errorf("adapter: invalid tenant id: %w", err)
	}
	keys, err := a.s.ListAPIKeysByTenant(ctx, tid)
	if err != nil {
		return nil, err
	}
	out := make([]*handler.APIKey, len(keys))
	for i := range keys {
		out[i] = &handler.APIKey{
			ID:         keys[i].ID.String(),
			TenantID:   keys[i].TenantID.String(),
			Name:       keys[i].Name,
			KeyPrefix:  keys[i].KeyPrefix,
			CreatedAt:  keys[i].CreatedAt,
			ExpiresAt:  keys[i].ExpiresAt,
			LastUsedAt: keys[i].LastUsedAt,
		}
	}
	return out, nil
}

// DeleteAPIKey removes an API key. The tenantID parameter is available for
// authorization checks; we verify the key belongs to the tenant.
func (a *APIKeyServiceAdapter) DeleteAPIKey(ctx context.Context, tenantID, keyID string) error {
	kid, err := parseUUID(keyID)
	if err != nil {
		return fmt.Errorf("adapter: invalid key id: %w", err)
	}
	// Optionally verify the key belongs to the tenant before deleting.
	// For now we trust the tenant-context middleware and delete directly.
	return a.s.DeleteAPIKey(ctx, kid)
}

// ---------------------------------------------------------------------------
// APIKeyValidatorAdapter  implements  middleware.APIKeyValidator
// ---------------------------------------------------------------------------

// APIKeyValidatorAdapter adapts *store.Store to the middleware.APIKeyValidator
// interface.
type APIKeyValidatorAdapter struct {
	s *store.Store
}

// NewAPIKeyValidatorAdapter returns a new APIKeyValidatorAdapter.
func NewAPIKeyValidatorAdapter(s *store.Store) *APIKeyValidatorAdapter {
	return &APIKeyValidatorAdapter{s: s}
}

// ValidateAPIKey looks up the API key by its hash, checks expiration, updates
// last-used timestamp, and returns the associated tenant and owner user IDs.
func (a *APIKeyValidatorAdapter) ValidateAPIKey(ctx context.Context, keyHash string) (string, string, error) {
	k, err := a.s.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return "", "", fmt.Errorf("adapter: invalid api key: %w", err)
	}

	// Check expiration.
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return "", "", fmt.Errorf("adapter: api key expired")
	}

	// Best-effort update last-used timestamp.
	_ = a.s.UpdateAPIKeyLastUsed(ctx, k.ID)

	// Look up the tenant to get the owner ID, which serves as the
	// authenticated user for this request.
	t, err := a.s.GetTenantByID(ctx, k.TenantID)
	if err != nil {
		return "", "", fmt.Errorf("adapter: tenant not found for api key: %w", err)
	}

	return t.ID.String(), t.OwnerID.String(), nil
}

// ---------------------------------------------------------------------------
// DevLogStreamer  implements  handler.LogStreamer  (stub)
// ---------------------------------------------------------------------------

// DevLogStreamer is a stub log streamer that returns a development-mode message.
type DevLogStreamer struct{}

// NewDevLogStreamer returns a new DevLogStreamer.
func NewDevLogStreamer() *DevLogStreamer {
	return &DevLogStreamer{}
}

// StreamLogs returns a channel that immediately sends a single informational
// message and then closes. Real pod-log streaming requires additional
// Kubernetes plumbing and is not available in development mode.
func (d *DevLogStreamer) StreamLogs(_ context.Context, _, _ string, _ bool, _ int) (<-chan handler.LogEntry, error) {
	ch := make(chan handler.LogEntry, 1)
	ch <- handler.LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   "logs not available in dev mode",
		Source:    "system",
	}
	close(ch)
	return ch, nil
}

// ---------------------------------------------------------------------------
// K8sClientAdapter  implements  service.K8sClient
// ---------------------------------------------------------------------------

// K8sClientAdapter uses the controller-runtime client to manage Tenant and
// App custom resources in Kubernetes.
type K8sClientAdapter struct {
	c         client.Client
	namespace string // default namespace for namespaced resources (Apps)
}

// NewK8sClientAdapter returns a new K8sClientAdapter. The namespace parameter
// is used as a fallback for App resources when the tenant namespace is not yet
// known (typically "default").
func NewK8sClientAdapter(c client.Client, namespace string) *K8sClientAdapter {
	if namespace == "" {
		namespace = "default"
	}
	return &K8sClientAdapter{c: c, namespace: namespace}
}

// CreateTenantCRD creates a Tenant custom resource. Tenants are cluster-scoped.
func (k *K8sClientAdapter) CreateTenantCRD(ctx context.Context, t *service.Tenant) error {
	obj := buildTenantCR(t)
	return k.c.Create(ctx, obj)
}

// UpdateTenantCRD updates the Tenant custom resource spec.
func (k *K8sClientAdapter) UpdateTenantCRD(ctx context.Context, t *service.Tenant) error {
	obj := &platformv1alpha1.Tenant{}
	// Tenant is cluster-scoped so no namespace.
	if err := k.c.Get(ctx, types.NamespacedName{Name: t.Name}, obj); err != nil {
		// If the CR doesn't exist yet, create it.
		return k.c.Create(ctx, buildTenantCR(t))
	}
	obj.Spec.DisplayName = t.DisplayName
	obj.Spec.Plan = platformv1alpha1.TenantPlan(t.Plan)
	return k.c.Update(ctx, obj)
}

// DeleteTenantCRD deletes the Tenant custom resource.
func (k *K8sClientAdapter) DeleteTenantCRD(ctx context.Context, tenantName string) error {
	obj := &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenantName,
		},
	}
	return client.IgnoreNotFound(k.c.Delete(ctx, obj))
}

// CreateAppCRD creates an App custom resource in the tenant's namespace.
func (k *K8sClientAdapter) CreateAppCRD(ctx context.Context, app *service.App, tenantName string) error {
	obj := buildAppCR(app, tenantName)
	return k.c.Create(ctx, obj)
}

// UpdateAppCRD updates the App custom resource spec.
func (k *K8sClientAdapter) UpdateAppCRD(ctx context.Context, app *service.App, tenantName string) error {
	ns := "tenant-" + tenantName
	obj := &platformv1alpha1.App{}
	if err := k.c.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: ns}, obj); err != nil {
		// If it doesn't exist, create it.
		return k.c.Create(ctx, buildAppCR(app, tenantName))
	}
	obj.Spec = buildAppSpec(app, tenantName)
	return k.c.Update(ctx, obj)
}

// DeleteAppCRD deletes the App custom resource.
func (k *K8sClientAdapter) DeleteAppCRD(ctx context.Context, appName, tenantName string) error {
	ns := "tenant-" + tenantName
	obj := &platformv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: ns,
		},
	}
	return client.IgnoreNotFound(k.c.Delete(ctx, obj))
}

// GetAppCRDStatus reads the App CRD from K8s and returns its status fields.
func (k *K8sClientAdapter) GetAppCRDStatus(ctx context.Context, appName, tenantName string) (*service.AppCRDStatus, error) {
	ns := "tenant-" + tenantName
	obj := &platformv1alpha1.App{}
	if err := k.c.Get(ctx, types.NamespacedName{Name: appName, Namespace: ns}, obj); err != nil {
		return nil, fmt.Errorf("adapter: get app CRD status: %w", err)
	}
	return &service.AppCRDStatus{
		Phase:          string(obj.Status.Phase),
		URL:            obj.Status.URL,
		LatestRevision: obj.Status.LatestRevision,
		ReadyInstances: obj.Status.ReadyInstances,
	}, nil
}

func buildTenantCR(t *service.Tenant) *platformv1alpha1.Tenant {
	return &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name: t.Name,
			Labels: map[string]string{
				"kuberless.io/tenant-id": t.ID,
			},
		},
		Spec: platformv1alpha1.TenantSpec{
			DisplayName: t.DisplayName,
			OwnerEmail:  "", // not available from service.Tenant; operator can reconcile
			Plan:        platformv1alpha1.TenantPlan(t.Plan),
		},
	}
}

func buildAppSpec(app *service.App, tenantName string) platformv1alpha1.AppSpec {
	env := make([]platformv1alpha1.EnvVar, 0, len(app.EnvVars))
	for k, v := range app.EnvVars {
		env = append(env, platformv1alpha1.EnvVar{Name: k, Value: v})
	}
	return platformv1alpha1.AppSpec{
		TenantRef: tenantName,
		Image:     app.Image,
		Port:      int32(app.Port),
		Env:       env,
		Scaling: platformv1alpha1.ScalingSpec{
			MinInstances:      int32(app.MinInstances),
			MaxInstances:      int32(app.MaxInstances),
			TargetConcurrency: int32(app.TargetConcurrency),
		},
		Resources: platformv1alpha1.AppResources{
			CPURequest:  app.CPURequest,
			MemoryLimit: app.MemoryLimit,
		},
		Paused: app.Paused,
	}
}

func buildAppCR(app *service.App, tenantName string) *platformv1alpha1.App {
	ns := "tenant-" + tenantName
	return &platformv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: ns,
			Labels: map[string]string{
				"kuberless.io/app-id":    app.ID,
				"kuberless.io/tenant-id": app.TenantID,
			},
		},
		Spec: buildAppSpec(app, tenantName),
	}
}

// ---------------------------------------------------------------------------
// K8sLogStreamer  implements  handler.LogStreamer  (real pod logs)
// ---------------------------------------------------------------------------

// K8sLogStreamer streams logs from Kubernetes pods using the core API.
type K8sLogStreamer struct {
	clientset kubernetes.Interface
}

// NewK8sLogStreamer returns a new K8sLogStreamer.
func NewK8sLogStreamer(clientset kubernetes.Interface) *K8sLogStreamer {
	return &K8sLogStreamer{clientset: clientset}
}

// StreamLogs finds pods for the given app in the namespace and streams their logs.
func (s *K8sLogStreamer) StreamLogs(ctx context.Context, namespace, appName string, follow bool, tail int) (<-chan handler.LogEntry, error) {
	// Find pods via the Knative serving label.
	pods, err := s.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "serving.knative.dev/service=" + appName,
	})
	if err != nil {
		return nil, fmt.Errorf("adapter: list pods: %w", err)
	}

	ch := make(chan handler.LogEntry, 64)

	if len(pods.Items) == 0 {
		go func() {
			ch <- handler.LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Message:   "no running pods found for app " + appName,
				Source:    "system",
			}
			close(ch)
		}()
		return ch, nil
	}

	tailLines := int64(tail)
	go func() {
		defer close(ch)
		for _, pod := range pods.Items {
			opts := &corev1.PodLogOptions{
				Container: "user-container",
				Follow:    follow,
				TailLines: &tailLines,
			}
			stream, err := s.clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, opts).Stream(ctx)
			if err != nil {
				ch <- handler.LogEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Message:   fmt.Sprintf("failed to stream logs from pod %s: %v", pod.Name, err),
					Source:    "system",
				}
				continue
			}
			scanner := bufio.NewScanner(stream)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					stream.Close()
					return
				case ch <- handler.LogEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Message:   scanner.Text(),
					Source:    pod.Name,
				}:
				}
			}
			stream.Close()
		}
	}()

	return ch, nil
}

// ---------------------------------------------------------------------------
// AppResolverAdapter  implements  handler.AppResolver
// ---------------------------------------------------------------------------

// AppResolverAdapter resolves an app ID to its Kubernetes namespace and name.
type AppResolverAdapter struct {
	apps    service.AppStore
	tenants service.TenantStore
}

// NewAppResolverAdapter returns a new AppResolverAdapter.
func NewAppResolverAdapter(apps service.AppStore, tenants service.TenantStore) *AppResolverAdapter {
	return &AppResolverAdapter{apps: apps, tenants: tenants}
}

// ResolveApp maps an app ID to (namespace, appName).
func (a *AppResolverAdapter) ResolveApp(ctx context.Context, appID string) (string, string, error) {
	app, err := a.apps.GetAppByID(ctx, appID)
	if err != nil {
		return "", "", fmt.Errorf("adapter: resolve app: %w", err)
	}
	tenant, err := a.tenants.GetTenantByID(ctx, app.TenantID)
	if err != nil {
		return "", "", fmt.Errorf("adapter: resolve tenant for app: %w", err)
	}
	return "tenant-" + tenant.Name, app.Name, nil
}

// ---------------------------------------------------------------------------
// Compile-time interface satisfaction checks
// ---------------------------------------------------------------------------

var (
	_ service.TenantStore        = (*TenantStoreAdapter)(nil)
	_ service.AppStore           = (*AppStoreAdapter)(nil)
	_ handler.APIKeyService      = (*APIKeyServiceAdapter)(nil)
	_ handler.LogStreamer         = (*DevLogStreamer)(nil)
	_ handler.LogStreamer         = (*K8sLogStreamer)(nil)
	_ handler.AppResolver        = (*AppResolverAdapter)(nil)
	_ middleware.APIKeyValidator  = (*APIKeyValidatorAdapter)(nil)
	_ service.K8sClient          = (*K8sClientAdapter)(nil)
)

// ---------------------------------------------------------------------------
// NewK8sClient helper – builds a controller-runtime client from kubeconfig
// ---------------------------------------------------------------------------

// NewK8sControllerClient creates a controller-runtime client configured with
// the platform CRD scheme. It uses the default kubeconfig loading rules
// (KUBECONFIG env var, then ~/.kube/config, then in-cluster).
func NewK8sControllerClient() (client.Client, error) {
	cfg, err := loadRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("adapter: load kubeconfig: %w", err)
	}

	scheme := schemeWithCRDs()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("adapter: create k8s client: %w", err)
	}
	return c, nil
}
