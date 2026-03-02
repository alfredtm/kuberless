package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// APIKeyValidator validates a hashed API key and returns the associated tenant
// and user IDs.
type APIKeyValidator interface {
	ValidateAPIKey(ctx context.Context, keyHash string) (tenantID, userID string, err error)
}

// APIKeyAuth returns an HTTP middleware that authenticates requests using an
// API key. The key can be provided via the X-API-Key header or as a Bearer
// token that starts with "sk-". On success the middleware stores the resolved
// user ID and tenant ID in the request context.
func APIKeyAuth(validator APIKeyValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := extractAPIKey(r)
			if apiKey == "" {
				// No API key found; pass through to allow other auth middleware.
				next.ServeHTTP(w, r)
				return
			}

			if validator == nil {
				http.Error(w, `{"error":"API key authentication not configured"}`, http.StatusUnauthorized)
				return
			}

			hash := hashAPIKey(apiKey)
			tenantID, userID, err := validator.ValidateAPIKey(r.Context(), hash)
			if err != nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, userIDKey, userID)
			ctx = context.WithValue(ctx, tenantIDKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractAPIKey attempts to find an API key in the request headers.
// It checks X-API-Key first, then falls back to the Authorization header
// for Bearer tokens that start with "sk-".
func extractAPIKey(r *http.Request) string {
	// Check X-API-Key header.
	if key := r.Header.Get("X-API-Key"); key != "" && strings.HasPrefix(key, "sk-") {
		return key
	}

	// Check Authorization: Bearer sk-... header.
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	token := parts[1]
	if strings.HasPrefix(token, "sk-") {
		return token
	}
	return ""
}

// hashAPIKey returns the hex-encoded SHA-256 hash of the given API key.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
