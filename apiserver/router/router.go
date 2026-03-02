package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/alfredtm/kuberless/apiserver/handler"
	"github.com/alfredtm/kuberless/apiserver/middleware"
)

// Config holds all the dependencies needed to build the router.
type Config struct {
	// Middleware configuration.
	JWTConfig       middleware.JWTConfig
	OIDCConfig      middleware.OIDCConfig
	RateLimitConfig middleware.RateLimitConfig
	APIKeyValidator middleware.APIKeyValidator

	// Tenant access checker (typically the TenantService).
	TenantAccessChecker middleware.TenantAccessChecker

	// Handlers.
	AdminAuthHandler *handler.AdminAuthHandler
	TenantHandler    *handler.TenantHandler
	AppHandler       *handler.AppHandler
	LogHandler       *handler.LogHandler
	APIKeyHandler    *handler.APIKeyHandler
}

// NewRouter creates and configures a chi.Mux with all API routes.
func NewRouter(cfg *Config) *chi.Mux {
	r := chi.NewRouter()

	// ----- Global middleware chain -----
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		ExposedHeaders: []string{"Link"},
		MaxAge:         300,
	}))
	r.Use(middleware.RateLimit(cfg.RateLimitConfig))

	// ----- Health check -----
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	r.Route("/api/v1", func(r chi.Router) {
		// ----- Public auth routes -----
		r.Route("/auth", func(r chi.Router) {
			// Admin login and Keycloak config (public, no auth middleware).
			if cfg.AdminAuthHandler != nil {
				r.Get("/config", cfg.AdminAuthHandler.HandleAuthConfig)
				r.Post("/admin-login", cfg.AdminAuthHandler.HandleAdminLogin)
			}
		})

		// ----- Authenticated routes -----
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware(cfg))

			// Tenant list and create (no tenant context needed).
			r.Post("/tenants", cfg.TenantHandler.HandleCreateTenant)
			r.Get("/tenants", cfg.TenantHandler.HandleListTenants)

			// Tenant-scoped routes (require tenant context middleware).
			r.Route("/tenants/{tid}", func(r chi.Router) {
				r.Use(middleware.TenantContext(cfg.TenantAccessChecker))

				r.Get("/", cfg.TenantHandler.HandleGetTenant)
				r.Put("/", cfg.TenantHandler.HandleUpdateTenant)
				r.Delete("/", cfg.TenantHandler.HandleDeleteTenant)

				// App routes.
				r.Route("/apps", func(r chi.Router) {
					r.Post("/", cfg.AppHandler.HandleCreateApp)
					r.Get("/", cfg.AppHandler.HandleListApps)

					r.Route("/{appID}", func(r chi.Router) {
						r.Get("/", cfg.AppHandler.HandleGetApp)
						r.Put("/", cfg.AppHandler.HandleUpdateApp)
						r.Delete("/", cfg.AppHandler.HandleDeleteApp)
						r.Post("/redeploy", cfg.AppHandler.HandleRedeployApp)

						// Env routes.
						r.Get("/env", cfg.AppHandler.HandleGetEnv)
						r.Put("/env", cfg.AppHandler.HandleUpdateEnv)
						r.Patch("/env", cfg.AppHandler.HandlePatchEnv)

						// Domain routes.
						r.Get("/domains", cfg.AppHandler.HandleListDomains)
						r.Post("/domains", cfg.AppHandler.HandleAddDomain)
						r.Delete("/domains/{hostname}", cfg.AppHandler.HandleRemoveDomain)

						// Log routes.
						if cfg.LogHandler != nil {
							r.Get("/logs", cfg.LogHandler.HandleGetLogs)
						}
					})
				})

				// API key routes.
				r.Route("/apikeys", func(r chi.Router) {
					r.Post("/", cfg.APIKeyHandler.HandleCreateAPIKey)
					r.Get("/", cfg.APIKeyHandler.HandleListAPIKeys)
					r.Delete("/{keyID}", cfg.APIKeyHandler.HandleDeleteAPIKey)
				})
			})
		})
	})

	return r
}

// authMiddleware returns a middleware that tries API key auth first, then
// OIDC (if enabled), then falls back to JWT auth.
func authMiddleware(cfg *Config) func(http.Handler) http.Handler {
	jwtMW := middleware.JWTAuth(cfg.JWTConfig)
	apiKeyMW := middleware.APIKeyAuth(cfg.APIKeyValidator)
	oidcMW := middleware.OIDCAuth(cfg.OIDCConfig)

	return func(next http.Handler) http.Handler {
		// The API key middleware is non-blocking: if no API key is present it
		// passes through. We chain it before OIDC and JWT so that requests
		// with a valid API key skip other validation.
		return apiKeyMW(oidcMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If APIKeyAuth or OIDCAuth already set a user ID, skip JWT.
			if _, err := middleware.UserIDFromContext(r.Context()); err == nil {
				next.ServeHTTP(w, r)
				return
			}
			// Otherwise, require JWT.
			jwtMW(next).ServeHTTP(w, r)
		})))
	}
}
