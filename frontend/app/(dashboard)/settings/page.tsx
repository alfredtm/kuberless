"use client";

import { useEffect, useState } from "react";
import * as api from "@/lib/api";

export default function SettingsPage() {
  const [tenant, setTenant] = useState<api.Tenant | null>(null);
  const [apiKeys, setApiKeys] = useState<api.APIKey[]>([]);
  const [newKeyName, setNewKeyName] = useState("");
  const [createdKey, setCreatedKey] = useState<string | null>(null);

  useEffect(() => {
    const tid = localStorage.getItem("tenant_id");
    if (tid) {
      api.getTenant(tid).then(setTenant).catch(() => {});
      api.listAPIKeys().then(setApiKeys).catch(() => {});
    }
  }, []);

  async function handleCreateKey() {
    if (!newKeyName) return;
    const key = await api.createAPIKey(newKeyName);
    setCreatedKey(key.key || null);
    setNewKeyName("");
    api.listAPIKeys().then(setApiKeys);
  }

  async function handleDeleteKey(id: string) {
    if (!confirm("Delete this API key?")) return;
    await api.deleteAPIKey(id);
    setApiKeys(apiKeys.filter((k) => k.id !== id));
  }

  return (
    <div className="space-y-8">
      <h2 className="text-2xl font-bold">Settings</h2>

      {/* Tenant Info */}
      {tenant && (
        <section className="rounded-lg border border-gray-200 bg-white p-6">
          <h3 className="text-lg font-semibold">Workspace</h3>
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <div>
              <p className="text-sm text-gray-500">Name</p>
              <p className="font-medium">{tenant.name}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Display Name</p>
              <p className="font-medium">{tenant.display_name}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Plan</p>
              <p className="font-medium capitalize">{tenant.plan}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500">Created</p>
              <p className="font-medium">{tenant.created_at}</p>
            </div>
          </div>
        </section>
      )}

      {/* API Keys */}
      <section className="rounded-lg border border-gray-200 bg-white p-6">
        <h3 className="text-lg font-semibold">API Keys</h3>
        <p className="mt-1 text-sm text-gray-500">
          Use API keys for programmatic access to the platform.
        </p>

        {createdKey && (
          <div className="mt-4 rounded-md bg-green-50 p-4">
            <p className="text-sm font-medium text-green-800">
              API key created. Copy it now — it won&apos;t be shown again.
            </p>
            <code className="mt-2 block break-all rounded bg-white p-2 font-mono text-sm">
              {createdKey}
            </code>
            <button
              onClick={() => setCreatedKey(null)}
              className="mt-2 text-sm text-green-700 hover:underline"
            >
              Dismiss
            </button>
          </div>
        )}

        <div className="mt-4 flex gap-2">
          <input
            placeholder="Key name (e.g. ci-deploy)"
            value={newKeyName}
            onChange={(e) => setNewKeyName(e.target.value)}
            className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
          />
          <button
            onClick={handleCreateKey}
            disabled={!newKeyName}
            className="rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Create Key
          </button>
        </div>

        {apiKeys.length > 0 && (
          <div className="mt-4 overflow-hidden rounded-lg border border-gray-200">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                    Name
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                    Prefix
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                    Created
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                    Expires
                  </th>
                  <th className="px-4 py-2" />
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {apiKeys.map((key) => (
                  <tr key={key.id}>
                    <td className="px-4 py-2 text-sm font-medium">{key.name}</td>
                    <td className="px-4 py-2 font-mono text-sm text-gray-500">
                      {key.key_prefix}...
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-500">
                      {key.created_at}
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-500">
                      {key.expires_at || "Never"}
                    </td>
                    <td className="px-4 py-2 text-right">
                      <button
                        onClick={() => handleDeleteKey(key.id)}
                        className="text-sm text-red-600 hover:underline"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
