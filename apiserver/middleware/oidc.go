package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// OIDCConfig holds the configuration for the OIDC/Keycloak middleware.
type OIDCConfig struct {
	Enabled   bool
	IssuerURL string
	ClientID  string
}

type oidcMiddleware struct {
	cfg     OIDCConfig
	jwksURL string
	keySet  jwk.Set
	mu      sync.RWMutex
}

// OIDCAuth returns an HTTP middleware that validates OIDC Bearer tokens
// against a Keycloak JWKS endpoint. If Keycloak is not enabled or the token
// is not a valid OIDC token, the request passes through to the next handler
// so that existing JWT auth can try.
func OIDCAuth(cfg OIDCConfig) func(http.Handler) http.Handler {
	m := &oidcMiddleware{cfg: cfg}

	if cfg.Enabled && cfg.IssuerURL != "" {
		go m.fetchJWKS() //nolint:errcheck
	}

	return m.handler
}

func (m *oidcMiddleware) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// If user is already authenticated (e.g. by API key), pass through.
		if _, err := UserIDFromContext(r.Context()); err == nil {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			next.ServeHTTP(w, r)
			return
		}

		tokenStr := parts[1]

		// Skip API keys.
		if strings.HasPrefix(tokenStr, "sk-") {
			next.ServeHTTP(w, r)
			return
		}

		userID, err := m.validateToken(r.Context(), tokenStr)
		if err != nil {
			// Not a valid OIDC token - fall through to let JWT middleware try.
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *oidcMiddleware) validateToken(ctx context.Context, tokenStr string) (string, error) {
	keySet, err := m.getKeySet(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get JWKS: %w", err)
	}

	// Verify the JWS signature using the JWKS.
	_, err = jws.Verify([]byte(tokenStr), jws.WithKeySet(keySet))
	if err != nil {
		// Try refreshing keys in case of key rotation.
		keySet, refreshErr := m.refreshKeySet(ctx)
		if refreshErr != nil {
			return "", fmt.Errorf("failed to verify token: %w", err)
		}
		if _, err = jws.Verify([]byte(tokenStr), jws.WithKeySet(keySet)); err != nil {
			return "", fmt.Errorf("failed to verify token after refresh: %w", err)
		}
	}

	// Parse and validate the JWT claims.
	tok, err := jwt.Parse([]byte(tokenStr), jwt.WithVerify(false), jwt.WithValidate(true))
	if err != nil {
		return "", fmt.Errorf("failed to parse token claims: %w", err)
	}

	sub := tok.Subject()
	if sub == "" {
		return "", fmt.Errorf("missing subject claim")
	}

	return sub, nil
}

func (m *oidcMiddleware) getKeySet(ctx context.Context) (jwk.Set, error) {
	m.mu.RLock()
	ks := m.keySet
	m.mu.RUnlock()

	if ks != nil {
		return ks, nil
	}

	return m.refreshKeySet(ctx)
}

func (m *oidcMiddleware) refreshKeySet(ctx context.Context) (jwk.Set, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.fetchJWKS(); err != nil {
		return nil, err
	}
	return m.keySet, nil
}

func (m *oidcMiddleware) fetchJWKS() error {
	if m.jwksURL == "" {
		jwksURL, err := m.discoverJWKSURL()
		if err != nil {
			return err
		}
		m.jwksURL = jwksURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ks, err := jwk.Fetch(ctx, m.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS from %s: %w", m.jwksURL, err)
	}

	m.keySet = ks
	return nil
}

func (m *oidcMiddleware) discoverJWKSURL() (string, error) {
	discoveryURL := strings.TrimSuffix(m.cfg.IssuerURL, "/") + "/.well-known/openid-configuration"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC discovery: %w", err)
	}
	defer resp.Body.Close()

	var discovery struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return "", fmt.Errorf("failed to decode OIDC discovery: %w", err)
	}

	if discovery.JWKSURI == "" {
		return "", fmt.Errorf("empty jwks_uri in OIDC discovery")
	}

	return discovery.JWKSURI, nil
}
