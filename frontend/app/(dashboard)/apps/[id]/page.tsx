"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import * as api from "@/lib/api";

function PhaseBadge({ phase }: { phase: string }) {
  const colors: Record<string, string> = {
    Ready: "bg-green-100 text-green-800",
    Deploying: "bg-yellow-100 text-yellow-800",
    Pending: "bg-gray-100 text-gray-800",
    Failed: "bg-red-100 text-red-800",
    Paused: "bg-blue-100 text-blue-800",
  };

  return (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
        colors[phase] || "bg-gray-100 text-gray-800"
      }`}
    >
      {phase}
    </span>
  );
}

export default function AppDetailPage() {
  const params = useParams();
  const router = useRouter();
  const appId = params.id as string;

  const [app, setApp] = useState<api.App | null>(null);
  const [env, setEnv] = useState<Record<string, string>>({});
  const [domains, setDomains] = useState<api.Domain[]>([]);
  const [logs, setLogs] = useState<string[]>([]);
  const [newDomain, setNewDomain] = useState("");
  const [newEnvKey, setNewEnvKey] = useState("");
  const [newEnvVal, setNewEnvVal] = useState("");
  const [tab, setTab] = useState<"overview" | "env" | "domains" | "logs">("overview");
  const logsEndRef = useRef<HTMLDivElement>(null);
  const stopLogsRef = useRef<(() => void) | null>(null);

  const loadApp = useCallback(() => {
    api.getApp(appId).then(setApp).catch(() => {});
  }, [appId]);

  useEffect(() => {
    loadApp();
  }, [appId, loadApp]);

  // Load env when switching to env tab
  useEffect(() => {
    if (tab === "env") {
      api.getEnv(appId).then(setEnv).catch(() => {});
    }
  }, [tab, appId]);

  // Load domains when switching to domains tab
  useEffect(() => {
    if (tab === "domains") {
      api.listDomains(appId).then(setDomains).catch(() => {});
    }
  }, [tab, appId]);

  // Auto-refresh app status every 5 seconds on overview tab
  useEffect(() => {
    if (tab === "overview") {
      const interval = setInterval(() => {
        api.getApp(appId).then(setApp).catch(() => {});
      }, 5000);
      return () => clearInterval(interval);
    }
  }, [tab, appId]);

  useEffect(() => {
    if (tab === "logs") {
      setLogs([]);
      const stop = api.streamLogs(appId, (line) => {
        setLogs((prev) => [...prev.slice(-500), line]);
      });
      stopLogsRef.current = stop;
      return () => stop();
    } else if (stopLogsRef.current) {
      stopLogsRef.current();
    }
  }, [tab, appId]);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  async function handleDelete() {
    if (!confirm("Are you sure you want to delete this app?")) return;
    await api.deleteApp(appId);
    router.push("/apps");
  }

  async function handleRedeploy() {
    await api.redeployApp(appId);
    loadApp();
  }

  async function handleTogglePause() {
    if (!app) return;
    await api.updateApp(appId, { paused: !app.paused });
    loadApp();
  }

  async function handleAddEnv() {
    if (!newEnvKey) return;
    const updated = { ...env, [newEnvKey]: newEnvVal };
    await api.setEnv(appId, updated);
    setEnv(updated);
    setNewEnvKey("");
    setNewEnvVal("");
  }

  async function handleRemoveEnv(key: string) {
    const updated = { ...env };
    delete updated[key];
    await api.setEnv(appId, updated);
    setEnv(updated);
  }

  async function handleAddDomain() {
    if (!newDomain) return;
    await api.addDomain(appId, newDomain);
    setDomains([...domains, { hostname: newDomain }]);
    setNewDomain("");
  }

  async function handleRemoveDomain(hostname: string) {
    await api.removeDomain(appId, hostname);
    setDomains(domains.filter((d) => d.hostname !== hostname));
  }

  if (!app) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-gray-300 border-t-blue-600" />
      </div>
    );
  }

  const tabs = [
    { key: "overview", label: "Overview" },
    { key: "env", label: "Environment" },
    { key: "domains", label: "Domains" },
    { key: "logs", label: "Logs" },
  ] as const;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold">{app.name}</h2>
          <p className="text-sm text-gray-500">{app.image}</p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleTogglePause}
            className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
          >
            {app.paused ? "Unpause" : "Pause"}
          </button>
          <button
            onClick={handleRedeploy}
            className="rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
          >
            Redeploy
          </button>
          <button
            onClick={handleDelete}
            className="rounded-md bg-red-600 px-3 py-1.5 text-sm text-white hover:bg-red-700"
          >
            Delete
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="mb-6 border-b border-gray-200">
        <div className="flex gap-4">
          {tabs.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`border-b-2 px-1 pb-3 text-sm font-medium ${
                tab === t.key
                  ? "border-blue-600 text-blue-600"
                  : "border-transparent text-gray-500 hover:text-gray-700"
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      {/* Overview */}
      {tab === "overview" && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-sm text-gray-500">Status</p>
            <div className="mt-1">
              <PhaseBadge phase={app.paused ? "Paused" : app.phase} />
            </div>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-sm text-gray-500">URL</p>
            {app.url ? (
              <a
                href={app.url}
                target="_blank"
                rel="noopener noreferrer"
                className="mt-1 block truncate text-blue-600 hover:underline"
              >
                {app.url}
              </a>
            ) : (
              <p className="mt-1 font-medium text-gray-400">N/A</p>
            )}
          </div>
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-sm text-gray-500">Port</p>
            <p className="mt-1 font-medium">{app.port}</p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-sm text-gray-500">Ready Instances</p>
            <p className="mt-1 font-medium">{app.ready_instances}</p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-sm text-gray-500">Latest Revision</p>
            <p className="mt-1 font-medium">{app.latest_revision || "N/A"}</p>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-sm text-gray-500">Created</p>
            <p className="mt-1 font-medium">{app.created_at}</p>
          </div>
        </div>
      )}

      {/* Environment */}
      {tab === "env" && (
        <div className="space-y-4">
          <div className="flex gap-2">
            <input
              placeholder="KEY"
              value={newEnvKey}
              onChange={(e) => setNewEnvKey(e.target.value)}
              className="w-48 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
            />
            <input
              placeholder="VALUE"
              value={newEnvVal}
              onChange={(e) => setNewEnvVal(e.target.value)}
              className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
            />
            <button
              onClick={handleAddEnv}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
            >
              Add
            </button>
          </div>
          <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
            {Object.entries(env).length === 0 ? (
              <p className="p-4 text-sm text-gray-500">No environment variables set.</p>
            ) : (
              <table className="min-w-full divide-y divide-gray-200">
                <tbody className="divide-y divide-gray-200">
                  {Object.entries(env).map(([key, val]) => (
                    <tr key={key}>
                      <td className="px-4 py-2 font-mono text-sm font-medium">{key}</td>
                      <td className="px-4 py-2 font-mono text-sm text-gray-600">{val}</td>
                      <td className="px-4 py-2 text-right">
                        <button
                          onClick={() => handleRemoveEnv(key)}
                          className="text-sm text-red-600 hover:underline"
                        >
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}

      {/* Domains */}
      {tab === "domains" && (
        <div className="space-y-4">
          <div className="flex gap-2">
            <input
              placeholder="example.com"
              value={newDomain}
              onChange={(e) => setNewDomain(e.target.value)}
              className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
            />
            <button
              onClick={handleAddDomain}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
            >
              Add Domain
            </button>
          </div>
          <div className="overflow-hidden rounded-lg border border-gray-200 bg-white">
            {domains.length === 0 ? (
              <p className="p-4 text-sm text-gray-500">No custom domains configured.</p>
            ) : (
              <ul className="divide-y divide-gray-200">
                {domains.map((d) => (
                  <li key={d.hostname} className="flex items-center justify-between px-4 py-3">
                    <a
                      href={`https://${d.hostname}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="font-mono text-sm text-blue-600 hover:underline"
                    >
                      {d.hostname}
                    </a>
                    <button
                      onClick={() => handleRemoveDomain(d.hostname)}
                      className="text-sm text-red-600 hover:underline"
                    >
                      Remove
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}

      {/* Logs */}
      {tab === "logs" && (
        <div className="overflow-hidden rounded-lg border border-gray-200 bg-black">
          <div className="h-[500px] overflow-y-auto p-4 font-mono text-xs text-green-400">
            {logs.length === 0 && (
              <p className="text-gray-500">Connecting...</p>
            )}
            {logs.map((line, i) => (
              <div key={i} className="whitespace-pre-wrap break-words">
                {line}
              </div>
            ))}
            <div ref={logsEndRef} />
          </div>
        </div>
      )}
    </div>
  );
}
