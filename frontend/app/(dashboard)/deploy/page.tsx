"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import * as api from "@/lib/api";

export default function DeployPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [port, setPort] = useState("8080");
  const [envPairs, setEnvPairs] = useState<{ key: string; value: string }[]>([]);
  const [error, setError] = useState("");
  const [deploying, setDeploying] = useState(false);

  function addEnvPair() {
    setEnvPairs([...envPairs, { key: "", value: "" }]);
  }

  function removeEnvPair(index: number) {
    setEnvPairs(envPairs.filter((_, i) => i !== index));
  }

  function updateEnvPair(index: number, field: "key" | "value", val: string) {
    const updated = [...envPairs];
    updated[index][field] = val;
    setEnvPairs(updated);
  }

  async function handleDeploy(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setDeploying(true);

    try {
      const envMap: Record<string, string> = {};
      for (const pair of envPairs) {
        if (pair.key) envMap[pair.key] = pair.value;
      }

      const app = await api.createApp({
        name: name || undefined!,
        image,
        port: parseInt(port, 10) || 8080,
        env: Object.keys(envMap).length > 0 ? envMap : undefined,
      });

      router.push(`/apps/${app.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Deployment failed");
    } finally {
      setDeploying(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="mb-6 text-2xl font-bold">Deploy New App</h2>

      <form onSubmit={handleDeploy} className="space-y-6">
        {error && (
          <div className="rounded-md bg-red-50 p-3 text-sm text-red-700">
            {error}
          </div>
        )}

        <div>
          <label className="mb-1 block text-sm font-medium">
            Container Image <span className="text-red-500">*</span>
          </label>
          <input
            value={image}
            onChange={(e) => setImage(e.target.value)}
            placeholder="ghcr.io/org/myapp:latest"
            className="w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            required
          />
          <p className="mt-1 text-xs text-gray-500">
            Full image reference including registry and tag
          </p>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium">App Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-app (auto-derived from image if empty)"
            className="w-full rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium">Port</label>
          <input
            type="number"
            value={port}
            onChange={(e) => setPort(e.target.value)}
            className="w-32 rounded-md border border-gray-300 px-3 py-2 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between">
            <label className="text-sm font-medium">Environment Variables</label>
            <button
              type="button"
              onClick={addEnvPair}
              className="text-sm text-blue-600 hover:underline"
            >
              + Add Variable
            </button>
          </div>
          {envPairs.map((pair, i) => (
            <div key={i} className="mb-2 flex gap-2">
              <input
                placeholder="KEY"
                value={pair.key}
                onChange={(e) => updateEnvPair(i, "key", e.target.value)}
                className="w-48 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
              />
              <input
                placeholder="VALUE"
                value={pair.value}
                onChange={(e) => updateEnvPair(i, "value", e.target.value)}
                className="flex-1 rounded-md border border-gray-300 px-3 py-1.5 text-sm"
              />
              <button
                type="button"
                onClick={() => removeEnvPair(i)}
                className="text-sm text-red-600 hover:underline"
              >
                Remove
              </button>
            </div>
          ))}
        </div>

        <button
          type="submit"
          disabled={deploying || !image}
          className="w-full rounded-md bg-blue-600 px-4 py-2 text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {deploying ? "Deploying..." : "Deploy"}
        </button>
      </form>
    </div>
  );
}
