package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alfredtm/kuberless/apiserver/middleware"
	"github.com/alfredtm/kuberless/apiserver/service"
)

// AppHandler holds HTTP handlers for app endpoints.
type AppHandler struct {
	appService *service.AppService
}

// NewAppHandler creates a new AppHandler.
func NewAppHandler(appService *service.AppService) *AppHandler {
	return &AppHandler{
		appService: appService,
	}
}

// HandleCreateApp handles POST /api/v1/tenants/{tid}/apps.
func (h *AppHandler) HandleCreateApp(w http.ResponseWriter, r *http.Request) {
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	var req service.CreateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Image == "" {
		respondError(w, http.StatusBadRequest, "image is required")
		return
	}

	app, err := h.appService.Create(r.Context(), tenantID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, app)
}

// HandleListApps handles GET /api/v1/tenants/{tid}/apps.
func (h *AppHandler) HandleListApps(w http.ResponseWriter, r *http.Request) {
	tenantID, err := middleware.TenantIDFromContext(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing tenant context")
		return
	}

	apps, err := h.appService.List(r.Context(), tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list apps")
		return
	}

	respondJSON(w, http.StatusOK, apps)
}

// HandleGetApp handles GET /api/v1/tenants/{tid}/apps/{appID}.
func (h *AppHandler) HandleGetApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	app, err := h.appService.Get(r.Context(), appID)
	if err != nil {
		respondError(w, http.StatusNotFound, "app not found")
		return
	}

	respondJSON(w, http.StatusOK, app)
}

// HandleUpdateApp handles PUT /api/v1/tenants/{tid}/apps/{appID}.
func (h *AppHandler) HandleUpdateApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	var req service.UpdateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	app, err := h.appService.Update(r.Context(), appID, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, app)
}

// HandleDeleteApp handles DELETE /api/v1/tenants/{tid}/apps/{appID}.
func (h *AppHandler) HandleDeleteApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	if err := h.appService.Delete(r.Context(), appID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRedeployApp handles POST /api/v1/tenants/{tid}/apps/{appID}/redeploy.
func (h *AppHandler) HandleRedeployApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	app, err := h.appService.Redeploy(r.Context(), appID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, app)
}

// HandleGetEnv handles GET /api/v1/tenants/{tid}/apps/{appID}/env.
func (h *AppHandler) HandleGetEnv(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	envVars, err := h.appService.GetEnv(r.Context(), appID)
	if err != nil {
		respondError(w, http.StatusNotFound, "app not found")
		return
	}

	respondJSON(w, http.StatusOK, envVars)
}

// HandleUpdateEnv handles PUT /api/v1/tenants/{tid}/apps/{appID}/env.
func (h *AppHandler) HandleUpdateEnv(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	var req service.UpdateEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	envVars, err := h.appService.UpdateEnv(r.Context(), appID, req.EnvVars)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, envVars)
}

// HandlePatchEnv handles PATCH /api/v1/tenants/{tid}/apps/{appID}/env.
func (h *AppHandler) HandlePatchEnv(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	var req service.PatchEnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	envVars, err := h.appService.PatchEnv(r.Context(), appID, req.EnvVars)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, envVars)
}

// HandleListDomains handles GET /api/v1/tenants/{tid}/apps/{appID}/domains.
func (h *AppHandler) HandleListDomains(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	domains, err := h.appService.ListDomains(r.Context(), appID)
	if err != nil {
		respondError(w, http.StatusNotFound, "app not found")
		return
	}

	respondJSON(w, http.StatusOK, domains)
}

// HandleAddDomain handles POST /api/v1/tenants/{tid}/apps/{appID}/domains.
func (h *AppHandler) HandleAddDomain(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	var req service.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Hostname == "" {
		respondError(w, http.StatusBadRequest, "hostname is required")
		return
	}

	domains, err := h.appService.AddDomain(r.Context(), appID, req.Hostname)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, domains)
}

// HandleRemoveDomain handles DELETE /api/v1/tenants/{tid}/apps/{appID}/domains/{hostname}.
func (h *AppHandler) HandleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		respondError(w, http.StatusBadRequest, "missing hostname")
		return
	}

	if err := h.appService.RemoveDomain(r.Context(), appID, hostname); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
