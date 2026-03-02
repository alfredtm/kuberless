package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"

	"github.com/alfredtm/kuberless/apiserver/adapter"
	"github.com/alfredtm/kuberless/apiserver/handler"
	"github.com/alfredtm/kuberless/apiserver/middleware"
	"github.com/alfredtm/kuberless/apiserver/router"
	"github.com/alfredtm/kuberless/apiserver/service"
	"github.com/alfredtm/kuberless/apiserver/store"
)

func main() {
	// ----- Configuration from environment variables -----
	databaseURL := envOrDefault("DATABASE_URL", "postgres://localhost:5432/kuberless?sslmode=disable")
	jwtSecret := envOrDefault("JWT_SECRET", "change-me-in-production")
	port := envOrDefault("PORT", "8080")
	// KUBECONFIG is read by the Kubernetes client libraries automatically.

	// Keycloak / OIDC configuration.
	keycloakEnabled := envOrDefault("KEYCLOAK_ENABLED", "false") == "true"
	keycloakIssuerURL := envOrDefault("KEYCLOAK_ISSUER_URL", "")
	keycloakClientID := envOrDefault("KEYCLOAK_CLIENT_ID", "kuberless")

	// Admin login configuration.
	adminLoginEnabled := envOrDefault("ADMIN_LOGIN_ENABLED", "true") == "true"
	adminUsername := envOrDefault("ADMIN_USERNAME", "")
	adminPassword := envOrDefault("ADMIN_PASSWORD", "")

	// ----- Database -----
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	if err := store.RunMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	log.Println("database migrations completed successfully")

	// Seed the admin user so the FK constraints on tenants/tenant_members are
	// satisfied when the admin creates workspaces. Uses a well-known UUID that
	// matches the sub claim issued by HandleAdminLogin.
	if adminLoginEnabled {
		_, err := db.Exec(
			`INSERT INTO users (id, email, password_hash, display_name, created_at, updated_at)
			 VALUES ($1, 'admin@kuberless.local', '', 'Admin', now(), now())
			 ON CONFLICT DO NOTHING`,
			handler.AdminUserID,
		)
		if err != nil {
			log.Printf("WARNING: failed to seed admin user: %v", err)
		}
	}

	// ----- Store -----
	pgStore := store.NewFromDB(db)

	// ----- Adapters -----
	// Bridge the *store.Store (Postgres, uuid.UUID IDs) to the service/handler/
	// middleware interfaces (string IDs, their own domain types).
	tenantStore := adapter.NewTenantStoreAdapter(pgStore)
	appStore := adapter.NewAppStoreAdapter(pgStore)
	apiKeyStore := adapter.NewAPIKeyServiceAdapter(pgStore)
	apiKeyValidator := adapter.NewAPIKeyValidatorAdapter(pgStore)
	// ----- Kubernetes client -----
	// Attempt to connect to Kubernetes for CRD management. If no cluster is
	// reachable (e.g. local development), the K8sClient is set to nil and the
	// service layer will skip CRD operations gracefully.
	var k8sClient service.K8sClient
	var logStreamer handler.LogStreamer
	k8sCR, k8sErr := adapter.NewK8sControllerClient()
	if k8sErr != nil {
		log.Printf("WARNING: Kubernetes client not available, CRD sync disabled: %v", k8sErr)
		logStreamer = adapter.NewDevLogStreamer()
	} else {
		k8sClient = adapter.NewK8sClientAdapter(k8sCR, "default")
		log.Println("Kubernetes client initialized, CRD sync enabled")

		// Use real pod log streaming when K8s is available.
		clientset, csErr := adapter.NewK8sClientset()
		if csErr != nil {
			log.Printf("WARNING: K8s clientset not available, using dev log streamer: %v", csErr)
			logStreamer = adapter.NewDevLogStreamer()
		} else {
			logStreamer = adapter.NewK8sLogStreamer(clientset)
			log.Println("K8s log streamer initialized")
		}
	}

	// ----- Services -----
	tenantService := service.NewTenantService(tenantStore, k8sClient)
	appService := service.NewAppService(appStore, tenantStore, k8sClient)

	// ----- Handlers -----
	adminAuthHandler := handler.NewAdminAuthHandler(handler.AdminAuthConfig{
		KeycloakEnabled:   keycloakEnabled,
		KeycloakIssuerURL: keycloakIssuerURL,
		KeycloakClientID:  keycloakClientID,
		AdminLoginEnabled: adminLoginEnabled,
		AdminUsername:     adminUsername,
		AdminPassword:     adminPassword,
		JWTSecret:         jwtSecret,
	})
	tenantHandler := handler.NewTenantHandler(tenantService)
	appHandler := handler.NewAppHandler(appService)
	appResolver := adapter.NewAppResolverAdapter(appStore, tenantStore)
	logHandler := handler.NewLogHandler(logStreamer, appResolver)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyStore)

	// ----- Router -----
	routerCfg := &router.Config{
		JWTConfig: middleware.JWTConfig{SecretKey: jwtSecret},
		OIDCConfig: middleware.OIDCConfig{
			Enabled:   keycloakEnabled,
			IssuerURL: keycloakIssuerURL,
			ClientID:  keycloakClientID,
		},
		RateLimitConfig:     middleware.DefaultRateLimitConfig(),
		APIKeyValidator:     apiKeyValidator,
		TenantAccessChecker: tenantService,
		AdminAuthHandler:    adminAuthHandler,
		TenantHandler:       tenantHandler,
		AppHandler:          appHandler,
		LogHandler:          logHandler,
		APIKeyHandler:       apiKeyHandler,
	}

	mux := router.NewRouter(routerCfg)

	// ----- Start server -----
	addr := fmt.Sprintf(":%s", port)
	log.Printf("API server starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// envOrDefault reads an environment variable or returns a default value.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
