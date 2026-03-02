package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Config holds CLI client configuration.
type Config struct {
	APIBaseURL  string `json:"api_base_url"`
	AccessToken string `json:"access_token"`
	TenantID    string `json:"tenant_id"`
	TenantName  string `json:"tenant_name"`
}

// Client is the HTTP client for the kuberless API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	config     *Config
}

// New creates a new API client.
func New() (*Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = &Config{
			APIBaseURL: "http://localhost:8080",
		}
	}

	return &Client{
		baseURL: cfg.APIBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: cfg,
	}, nil
}

// NewWithConfig creates a new API client with the given config.
func NewWithConfig(cfg *Config) *Client {
	return &Client{
		baseURL: cfg.APIBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: cfg,
	}
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kuberless")
}

func configPath() string {
	return filepath.Join(configDir(), "credentials.json")
}

// LoadConfig loads the CLI config from disk.
func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig saves the CLI config to disk.
func SaveConfig(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

// GetConfig returns the current config.
func (c *Client) GetConfig() *Config {
	return c.config
}

// SetTenant updates the active tenant in config.
func (c *Client) SetTenant(id, name string) error {
	c.config.TenantID = id
	c.config.TenantName = name
	return SaveConfig(c.config)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.config.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshaling response: %w", err)
		}
	}

	return nil
}

// Auth methods

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	} `json:"user"`
}

func (c *Client) AdminLogin(ctx context.Context, req *AdminLoginRequest) (*AuthResponse, error) {
	var resp AuthResponse
	if err := c.doRequest(ctx, "POST", "/api/v1/auth/admin-login", req, &resp); err != nil {
		return nil, err
	}
	c.config.AccessToken = resp.AccessToken
	return &resp, SaveConfig(c.config)
}

// Tenant methods

type CreateTenantRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Plan        string `json:"plan,omitempty"`
}

type TenantResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Plan        string `json:"plan"`
	CreatedAt   string `json:"created_at"`
}

func (c *Client) CreateTenant(ctx context.Context, req *CreateTenantRequest) (*TenantResponse, error) {
	var resp TenantResponse
	err := c.doRequest(ctx, "POST", "/api/v1/tenants", req, &resp)
	return &resp, err
}

func (c *Client) ListTenants(ctx context.Context) ([]TenantResponse, error) {
	var resp []TenantResponse
	err := c.doRequest(ctx, "GET", "/api/v1/tenants", nil, &resp)
	return resp, err
}

func (c *Client) GetTenant(ctx context.Context, id string) (*TenantResponse, error) {
	var resp TenantResponse
	err := c.doRequest(ctx, "GET", "/api/v1/tenants/"+id, nil, &resp)
	return &resp, err
}

func (c *Client) DeleteTenant(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", "/api/v1/tenants/"+id, nil, nil)
}

// App methods

type CreateAppRequest struct {
	Name  string            `json:"name"`
	Image string            `json:"image"`
	Port  int32             `json:"port,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
}

type AppResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Image          string `json:"image"`
	Port           int32  `json:"port"`
	Phase          string `json:"phase"`
	URL            string `json:"url"`
	LatestRevision string `json:"latest_revision"`
	ReadyInstances int32  `json:"ready_instances"`
	Paused         bool   `json:"paused"`
	CreatedAt      string `json:"created_at"`
}

func (c *Client) tenantPath() string {
	return "/api/v1/tenants/" + c.config.TenantID
}

func (c *Client) CreateApp(ctx context.Context, req *CreateAppRequest) (*AppResponse, error) {
	var resp AppResponse
	err := c.doRequest(ctx, "POST", c.tenantPath()+"/apps", req, &resp)
	return &resp, err
}

func (c *Client) ListApps(ctx context.Context) ([]AppResponse, error) {
	var resp []AppResponse
	err := c.doRequest(ctx, "GET", c.tenantPath()+"/apps", nil, &resp)
	return resp, err
}

func (c *Client) GetApp(ctx context.Context, appID string) (*AppResponse, error) {
	var resp AppResponse
	err := c.doRequest(ctx, "GET", c.tenantPath()+"/apps/"+appID, nil, &resp)
	return &resp, err
}

// GetAppByName finds an app by name, falling back to UUID lookup.
func (c *Client) GetAppByName(ctx context.Context, name string) (*AppResponse, error) {
	apps, err := c.ListApps(ctx)
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].Name == name {
			return &apps[i], nil
		}
	}
	// Fall back to treating name as an ID.
	return c.GetApp(ctx, name)
}

func (c *Client) DeleteApp(ctx context.Context, appID string) error {
	return c.doRequest(ctx, "DELETE", c.tenantPath()+"/apps/"+appID, nil, nil)
}

type UpdateAppRequest struct {
	Image  string `json:"image,omitempty"`
	Port   int32  `json:"port,omitempty"`
	Paused *bool  `json:"paused,omitempty"`
}

func (c *Client) UpdateApp(ctx context.Context, appID string, req *UpdateAppRequest) (*AppResponse, error) {
	var resp AppResponse
	err := c.doRequest(ctx, "PUT", c.tenantPath()+"/apps/"+appID, req, &resp)
	return &resp, err
}

func (c *Client) RedeployApp(ctx context.Context, appID string) (*AppResponse, error) {
	var resp AppResponse
	err := c.doRequest(ctx, "POST", c.tenantPath()+"/apps/"+appID+"/redeploy", nil, &resp)
	return &resp, err
}

// Env methods

func (c *Client) GetEnv(ctx context.Context, appID string) (map[string]string, error) {
	var resp map[string]string
	err := c.doRequest(ctx, "GET", c.tenantPath()+"/apps/"+appID+"/env", nil, &resp)
	return resp, err
}

func (c *Client) SetEnv(ctx context.Context, appID string, env map[string]string) error {
	return c.doRequest(ctx, "PUT", c.tenantPath()+"/apps/"+appID+"/env", env, nil)
}

func (c *Client) PatchEnv(ctx context.Context, appID string, env map[string]string) error {
	return c.doRequest(ctx, "PATCH", c.tenantPath()+"/apps/"+appID+"/env", env, nil)
}

// Domain methods

type DomainResponse struct {
	Hostname string `json:"hostname"`
}

func (c *Client) ListDomains(ctx context.Context, appID string) ([]DomainResponse, error) {
	var resp []DomainResponse
	err := c.doRequest(ctx, "GET", c.tenantPath()+"/apps/"+appID+"/domains", nil, &resp)
	return resp, err
}

func (c *Client) AddDomain(ctx context.Context, appID, hostname string) (*DomainResponse, error) {
	var resp DomainResponse
	err := c.doRequest(ctx, "POST", c.tenantPath()+"/apps/"+appID+"/domains", map[string]string{"hostname": hostname}, &resp)
	return &resp, err
}

func (c *Client) RemoveDomain(ctx context.Context, appID, hostname string) error {
	return c.doRequest(ctx, "DELETE", c.tenantPath()+"/apps/"+appID+"/domains/"+hostname, nil, nil)
}

// Log streaming

func (c *Client) StreamLogs(ctx context.Context, appID string, follow bool, tail int) (io.ReadCloser, error) {
	path := fmt.Sprintf("%s/apps/%s/logs?tail=%d", c.tenantPath(), appID, tail)
	if follow {
		path += "&follow=true"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error (%d)", resp.StatusCode)
	}

	return resp.Body, nil
}

// API Key methods

type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

type APIKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key,omitempty"` // Only returned on create
	KeyPrefix string `json:"key_prefix"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func (c *Client) CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*APIKeyResponse, error) {
	var resp APIKeyResponse
	err := c.doRequest(ctx, "POST", c.tenantPath()+"/apikeys", req, &resp)
	return &resp, err
}

func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKeyResponse, error) {
	var resp []APIKeyResponse
	err := c.doRequest(ctx, "GET", c.tenantPath()+"/apikeys", nil, &resp)
	return resp, err
}

func (c *Client) DeleteAPIKey(ctx context.Context, keyID string) error {
	return c.doRequest(ctx, "DELETE", c.tenantPath()+"/apikeys/"+keyID, nil, nil)
}
