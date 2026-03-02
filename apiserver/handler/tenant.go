package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alfredtm/kuberless/apiserver/middleware"
	"github.com/alfredtm/kuberless/apiserver/service"
)

// TenantHandler holds HTTP handlers for tenant endpoints.
type TenantHandler struct {
	tenantService *service.TenantService
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(tenantService *service.TenantService) *TenantHandler {
	return &TenantHandler{
		tenantService: tenantService,
	}
}

// HandleCreateTenant handles POST /api/v1/tenants.
func (h *TenantHandler) HandleCreateTenant(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req service.CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	tenant, err := h.tenantService.Create(r.Context(), userID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, tenant)
}

// HandleListTenants handles GET /api/v1/tenants.
func (h *TenantHandler) HandleListTenants(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	tenants, err := h.tenantService.List(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}

	respondJSON(w, http.StatusOK, tenants)
}

// HandleGetTenant handles GET /api/v1/tenants/{tid}.
func (h *TenantHandler) HandleGetTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tid")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "missing tenant ID")
		return
	}

	tenant, err := h.tenantService.Get(r.Context(), tenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "tenant not found")
		return
	}

	respondJSON(w, http.StatusOK, tenant)
}

// HandleUpdateTenant handles PUT /api/v1/tenants/{tid}.
func (h *TenantHandler) HandleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tid")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "missing tenant ID")
		return
	}

	var req service.UpdateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenant, err := h.tenantService.Update(r.Context(), tenantID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, tenant)
}

// HandleDeleteTenant handles DELETE /api/v1/tenants/{tid}.
func (h *TenantHandler) HandleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tid")
	if tenantID == "" {
		respondError(w, http.StatusBadRequest, "missing tenant ID")
		return
	}

	if err := h.tenantService.Delete(r.Context(), tenantID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
