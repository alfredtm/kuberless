package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthConfigResponse is returned by GET /api/v1/auth/config.
type AuthConfigResponse struct {
	KeycloakEnabled  bool   `json:"keycloak_enabled"`
	AdminLoginEnabled bool  `json:"admin_login_enabled"`
	KeycloakIssuerURL string `json:"keycloak_issuer_url"`
	KeycloakClientID  string `json:"keycloak_client_id"`
}

// AdminAuthConfig holds the configuration for admin authentication.
type AdminAuthConfig struct {
	KeycloakEnabled   bool
	KeycloakIssuerURL string
	KeycloakClientID  string
	AdminLoginEnabled bool
	AdminUsername     string
	AdminPassword     string
	JWTSecret        string
}

// AdminUserID is the well-known UUID used as the admin user's subject claim.
// It must match the row seeded into the users table on startup.
const AdminUserID = "00000000-0000-0000-0000-000000000001"

// AdminAuthHandler holds HTTP handlers for admin auth endpoints.
type AdminAuthHandler struct {
	cfg AdminAuthConfig
}

// NewAdminAuthHandler creates a new AdminAuthHandler.
func NewAdminAuthHandler(cfg AdminAuthConfig) *AdminAuthHandler {
	return &AdminAuthHandler{cfg: cfg}
}

// HandleAuthConfig handles GET /api/v1/auth/config.
func (h *AdminAuthHandler) HandleAuthConfig(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, AuthConfigResponse{
		KeycloakEnabled:   h.cfg.KeycloakEnabled,
		AdminLoginEnabled: h.cfg.AdminLoginEnabled,
		KeycloakIssuerURL: h.cfg.KeycloakIssuerURL,
		KeycloakClientID:  h.cfg.KeycloakClientID,
	})
}

// HandleAdminLogin handles POST /api/v1/auth/admin-login.
func (h *AdminAuthHandler) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.AdminLoginEnabled {
		respondError(w, http.StatusNotFound, "admin login is not enabled")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	usernameMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.cfg.AdminUsername)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.cfg.AdminPassword)) == 1

	if !usernameMatch || !passwordMatch {
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	now := time.Now()

	accessClaims := jwt.MapClaims{
		"sub":   AdminUserID,
		"email": "admin@kuberless.local",
		"role":  "admin",
		"type":  "access",
		"iat":   now.Unix(),
		"exp":   now.Add(15 * time.Minute).Unix(),
	}
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err := at.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": accessToken,
		"user": map[string]string{
			"id":           AdminUserID,
			"email":        "admin@kuberless.local",
			"display_name": "Admin",
		},
	})
}
