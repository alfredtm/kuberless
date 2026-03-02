package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const tenantIDKey contextKey = "tenantID"

// TenantAccessChecker verifies whether a user has access to a tenant.
type TenantAccessChecker interface {
	UserHasAccessToTenant(ctx context.Context, userID, tenantID string) (bool, error)
}

// TenantContext returns an HTTP middleware that extracts the tenant ID from the
// URL path parameter {tid}, verifies the authenticated user has access to that
// tenant, and stores the tenant ID in the request context.
func TenantContext(checker TenantAccessChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := chi.URLParam(r, "tid")
			if tenantID == "" {
				http.Error(w, `{"error":"missing tenant ID"}`, http.StatusBadRequest)
				return
			}

			userID, err := UserIDFromContext(r.Context())
			if err != nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			ok, err := checker.UserHasAccessToTenant(r.Context(), userID, tenantID)
			if err != nil {
				http.Error(w, `{"error":"failed to verify tenant access"}`, http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, `{"error":"access denied to tenant"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), tenantIDKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantIDFromContext extracts the tenant ID from the request context.
func TenantIDFromContext(ctx context.Context) (string, error) {
	tid, ok := ctx.Value(tenantIDKey).(string)
	if !ok || tid == "" {
		return "", errors.New("tenant ID not found in context")
	}
	return tid, nil
}
