package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alfredtm/kuberless/apiserver/middleware"
)

// APIKey represents an API key record returned to the client.
type APIKey struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenantId"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"keyPrefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// CreateAPIKeyResponse includes the full key (only shown once) plus metadata.
type CreateAPIKeyResponse struct {
	APIKey
	Key string `json:"key"`
}

// APIKeyService defines the interface for API key business logic.
type APIKeyService interface {
	CreateAPIKey(ctx context.Context, tenantID, name string) (*CreateAPIKeyResponse, error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]*APIKey, error)
	DeleteAPIKey(ctx context.Context, tenantID, keyID string) error
}

// APIKeyHandler holds HTTP handlers for API key endpoints.
type APIKeyHandler struct {
	service APIKeyService
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(service APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{
		service: service,
	}
}

// HandleCreateAPIKey handles POST /api/v1/tenants/{tid}/apikeys.
func (h *APIKeyHandler) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	resp, err := h.service.CreateAPIKey(r.Context(), tenantID, body.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, resp)
}

// HandleListAPIKeys handles GET /api/v1/tenants/{tid}/apikeys.
func (h *APIKeyHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	keys, err := h.service.ListAPIKeys(r.Context(), tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}

	respondJSON(w, http.StatusOK, keys)
}

// HandleDeleteAPIKey handles DELETE /api/v1/tenants/{tid}/apikeys/{keyID}.
func (h *APIKeyHandler) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	keyID := chi.URLParam(r, "keyID")
	if keyID == "" {
		respondError(w, http.StatusBadRequest, "missing key ID")
		return
	}

	if err := h.service.DeleteAPIKey(r.Context(), tenantID, keyID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
