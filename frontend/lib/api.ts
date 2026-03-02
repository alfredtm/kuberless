const API_BASE = "/api/v1";

async function fetchAPI<T>(
  path: string,
  options?: RequestInit
): Promise<T> {
  const token =
    typeof window !== "undefined" ? localStorage.getItem("token") : null;

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options?.headers,
    },
  });

  if (res.status === 401) {
    if (typeof window !== "undefined") {
      localStorage.removeItem("token");
      window.location.href = "/login";
    }
    throw new Error("Unauthorized");
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || "API request failed");
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

// Auth config (public, no auth required)
export interface AuthConfig {
  keycloak_enabled: boolean;
  admin_login_enabled: boolean;
  keycloak_issuer_url: string;
  keycloak_client_id: string;
}

export async function getAuthConfig(): Promise<AuthConfig> {
  const res = await fetch(`${API_BASE}/auth/config`);
  if (!res.ok) {
    throw new Error("Failed to fetch auth config");
  }
  return res.json();
}

export async function adminLogin(
  username: string,
  password: string
): Promise<AuthResponse> {
  const res = await fetchAPI<AuthResponse>("/auth/admin-login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
  localStorage.setItem("token", res.access_token);
  return res;
}

// Auth
export interface User {
  id: string;
  email: string;
  display_name: string;
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: User;
}

export function logout() {
  localStorage.removeItem("token");
  localStorage.removeItem("tenant_id");
  window.location.href = "/login";
}

// Tenants
export interface Tenant {
  id: string;
  name: string;
  display_name: string;
  plan: string;
  created_at: string;
}

export async function createTenant(
  name: string,
  displayName: string,
  plan: string
): Promise<Tenant> {
  return fetchAPI("/tenants", {
    method: "POST",
    body: JSON.stringify({ name, display_name: displayName, plan }),
  });
}

export async function listTenants(): Promise<Tenant[]> {
  return fetchAPI("/tenants");
}

export async function getTenant(id: string): Promise<Tenant> {
  return fetchAPI(`/tenants/${id}`);
}

export async function deleteTenant(id: string): Promise<void> {
  return fetchAPI(`/tenants/${id}`, { method: "DELETE" });
}

// Apps
export interface App {
  id: string;
  name: string;
  image: string;
  port: number;
  phase: string;
  url: string;
  latest_revision: string;
  ready_instances: number;
  paused: boolean;
  created_at: string;
}

function tenantPath(): string {
  const tid = localStorage.getItem("tenant_id") || "";
  return `/tenants/${tid}`;
}

export async function createApp(data: {
  name: string;
  image: string;
  port?: number;
  env?: Record<string, string>;
}): Promise<App> {
  return fetchAPI(`${tenantPath()}/apps`, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function listApps(): Promise<App[]> {
  return fetchAPI(`${tenantPath()}/apps`);
}

export async function getApp(id: string): Promise<App> {
  return fetchAPI(`${tenantPath()}/apps/${id}`);
}

export async function updateApp(
  id: string,
  data: Partial<{ image: string; port: number; paused: boolean }>
): Promise<App> {
  return fetchAPI(`${tenantPath()}/apps/${id}`, {
    method: "PATCH",
    body: JSON.stringify(data),
  });
}

export async function deleteApp(id: string): Promise<void> {
  return fetchAPI(`${tenantPath()}/apps/${id}`, { method: "DELETE" });
}

export async function redeployApp(id: string): Promise<App> {
  return fetchAPI(`${tenantPath()}/apps/${id}/redeploy`, { method: "POST" });
}

// Env
export async function getEnv(appId: string): Promise<Record<string, string>> {
  return fetchAPI(`${tenantPath()}/apps/${appId}/env`);
}

export async function setEnv(
  appId: string,
  env: Record<string, string>
): Promise<void> {
  return fetchAPI(`${tenantPath()}/apps/${appId}/env`, {
    method: "PUT",
    body: JSON.stringify(env),
  });
}

// Domains
export interface Domain {
  hostname: string;
}

export async function listDomains(appId: string): Promise<Domain[]> {
  return fetchAPI(`${tenantPath()}/apps/${appId}/domains`);
}

export async function addDomain(
  appId: string,
  hostname: string
): Promise<Domain> {
  return fetchAPI(`${tenantPath()}/apps/${appId}/domains`, {
    method: "POST",
    body: JSON.stringify({ hostname }),
  });
}

export async function removeDomain(
  appId: string,
  hostname: string
): Promise<void> {
  return fetchAPI(`${tenantPath()}/apps/${appId}/domains/${hostname}`, {
    method: "DELETE",
  });
}

// API Keys
export interface APIKey {
  id: string;
  name: string;
  key?: string;
  key_prefix: string;
  created_at: string;
  expires_at?: string;
}

export async function createAPIKey(name: string): Promise<APIKey> {
  return fetchAPI(`${tenantPath()}/apikeys`, {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export async function listAPIKeys(): Promise<APIKey[]> {
  return fetchAPI(`${tenantPath()}/apikeys`);
}

export async function deleteAPIKey(id: string): Promise<void> {
  return fetchAPI(`${tenantPath()}/apikeys/${id}`, { method: "DELETE" });
}

// Log streaming
export function streamLogs(
  appId: string,
  onData: (line: string) => void,
  tail = 100,
  follow = true
): () => void {
  const token = localStorage.getItem("token") || "";
  const url = `${API_BASE}${tenantPath()}/apps/${appId}/logs?tail=${tail}&follow=${follow}`;

  const controller = new AbortController();

  fetch(url, {
    headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" },
    signal: controller.signal,
  })
    .then(async (res) => {
      if (!res.ok || !res.body) return;
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";
        for (const line of lines) {
          if (line.startsWith("data: ")) {
            onData(line.slice(6));
          }
        }
      }
    })
    .catch(() => {});

  return () => controller.abort();
}
