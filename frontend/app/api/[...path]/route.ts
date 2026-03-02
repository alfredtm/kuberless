import { NextRequest } from "next/server";

// Read at request time (runtime) — not baked in at build time like next.config.ts rewrites.
const API_URL = process.env.API_URL || "http://localhost:8080";

async function proxy(req: NextRequest): Promise<Response> {
  const url = `${API_URL}${req.nextUrl.pathname}${req.nextUrl.search}`;

  // Forward only the headers the backend needs.
  const reqHeaders = new Headers();
  for (const name of ["content-type", "authorization", "accept"]) {
    const value = req.headers.get(name);
    if (value) reqHeaders.set(name, value);
  }

  const hasBody = req.method !== "GET" && req.method !== "HEAD";
  const upstream = await fetch(url, {
    method: req.method,
    headers: reqHeaders,
    body: hasBody ? await req.arrayBuffer() : undefined,
  });

  // Forward safe response headers (including content-type for SSE streaming).
  const resHeaders = new Headers();
  for (const name of ["content-type", "cache-control", "content-length"]) {
    const value = upstream.headers.get(name);
    if (value) resHeaders.set(name, value);
  }

  return new Response(upstream.body, {
    status: upstream.status,
    headers: resHeaders,
  });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
