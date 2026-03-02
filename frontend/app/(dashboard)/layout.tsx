"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import * as api from "@/lib/api";

const navItems = [
  { href: "/apps", label: "Apps", icon: "M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" },
  { href: "/deploy", label: "Deploy", icon: "M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" },
  { href: "/settings", label: "Settings", icon: "M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z" },
];

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [tenants, setTenants] = useState<api.Tenant[]>([]);
  const [activeTenant, setActiveTenant] = useState<string>("");
  const [loaded, setLoaded] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  useEffect(() => {
    const token = localStorage.getItem("token");
    if (!token) {
      router.push("/login");
      return;
    }
    api
      .listTenants()
      .then((t) => {
        setTenants(t);
        setLoaded(true);
        const stored = localStorage.getItem("tenant_id");
        if (t.length > 0 && (!stored || !t.find((x) => x.id === stored))) {
          localStorage.setItem("tenant_id", t[0].id);
          setActiveTenant(t[0].id);
        } else {
          setActiveTenant(stored || "");
        }
      })
      .catch(() => router.push("/login"));
  }, [router]);

  function handleTenantSwitch(tid: string) {
    localStorage.setItem("tenant_id", tid);
    setActiveTenant(tid);
    window.location.reload();
  }

  async function handleCreateTenant() {
    if (!newName) return;
    setCreating(true);
    setCreateError("");
    try {
      const t = await api.createTenant(newName, newName, "free");
      localStorage.setItem("tenant_id", t.id);
      window.location.reload();
    } catch (e: unknown) {
      setCreateError(e instanceof Error ? e.message : "Failed to create tenant");
    } finally {
      setCreating(false);
    }
  }

  if (loaded && tenants.length === 0 && !showCreate) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="w-full max-w-md space-y-6 rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
          <div>
            <h1 className="text-2xl font-bold">Welcome to Kuberless</h1>
            <p className="mt-2 text-gray-600">Create your first workspace to get started.</p>
          </div>
          <button
            onClick={() => setShowCreate(true)}
            className="w-full rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700"
          >
            Create Workspace
          </button>
          <button
            onClick={() => api.logout()}
            className="w-full text-sm text-gray-500 hover:text-gray-700"
          >
            Sign Out
          </button>
        </div>
      </div>
    );
  }

  if (showCreate || (loaded && tenants.length === 0)) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="w-full max-w-md space-y-4 rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
          <h2 className="text-xl font-bold">Create Workspace</h2>
          {createError && (
            <p className="rounded-md bg-red-50 p-3 text-sm text-red-600">{createError}</p>
          )}
          <input
            placeholder="Workspace name (e.g. my-org)"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-2"
            onKeyDown={(e) => e.key === "Enter" && handleCreateTenant()}
          />
          <button
            onClick={handleCreateTenant}
            disabled={!newName || creating}
            className="w-full rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {creating ? "Creating..." : "Create"}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen">
      {/* Sidebar */}
      <aside className="flex w-64 flex-col border-r border-gray-200 bg-white">
        <div className="border-b border-gray-200 p-4">
          <h1 className="text-lg font-bold">Kuberless</h1>
          {tenants.length > 0 && (
            <select
              value={activeTenant}
              onChange={(e) => handleTenantSwitch(e.target.value)}
              className="mt-2 w-full rounded-md border border-gray-300 px-2 py-1 text-sm"
            >
              {tenants.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.display_name}
                </option>
              ))}
            </select>
          )}
        </div>

        <nav className="flex-1 space-y-1 p-2">
          {navItems.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium ${
                  active
                    ? "bg-blue-50 text-blue-700"
                    : "text-gray-700 hover:bg-gray-100"
                }`}
              >
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
                </svg>
                {item.label}
              </Link>
            );
          })}
        </nav>

        <div className="border-t border-gray-200 p-4">
          <button
            onClick={() => api.logout()}
            className="text-sm text-gray-600 hover:text-gray-900"
          >
            Sign Out
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-y-auto p-8">{children}</main>
    </div>
  );
}
