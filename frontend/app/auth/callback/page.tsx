"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import * as api from "@/lib/api";

function CallbackHandler() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState("");

  useEffect(() => {
    const code = searchParams.get("code");
    if (!code) {
      setError("Missing authorization code");
      return;
    }

    async function exchangeCode(code: string) {
      try {
        const config = await api.getAuthConfig();
        if (!config.keycloak_enabled) {
          setError("Keycloak is not enabled");
          return;
        }

        const tokenUrl = `${config.keycloak_issuer_url}/protocol/openid-connect/token`;
        const redirectUri = `${window.location.origin}/auth/callback`;

        const body = new URLSearchParams({
          grant_type: "authorization_code",
          client_id: config.keycloak_client_id,
          code,
          redirect_uri: redirectUri,
        });

        const res = await fetch(tokenUrl, {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded" },
          body: body.toString(),
        });

        if (!res.ok) {
          setError("Failed to exchange authorization code");
          return;
        }

        const data = await res.json();
        if (data.access_token) {
          localStorage.setItem("token", data.access_token);
          router.push("/apps");
        } else {
          setError("No access token in response");
        }
      } catch {
        setError("Failed to complete authentication");
      }
    }

    exchangeCode(code);
  }, [searchParams, router]);

  if (error) {
    return (
      <div className="w-full max-w-md space-y-4 text-center">
        <h1 className="text-2xl font-bold">Authentication Error</h1>
        <p className="text-red-600">{error}</p>
        <a href="/login" className="text-blue-600 hover:underline">
          Back to login
        </a>
      </div>
    );
  }

  return <p className="text-gray-600">Completing authentication...</p>;
}

export default function AuthCallbackPage() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Suspense fallback={<p className="text-gray-600">Loading...</p>}>
        <CallbackHandler />
      </Suspense>
    </div>
  );
}
