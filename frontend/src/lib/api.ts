// Tiny typed fetch client. Dev server proxies /api -> Go server (:8080).

import { isDemo, demoResolve } from "./demo";
import { markLoggedOut } from "./auth";

const BASE = "/api";

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  // Demo mode (e.g. Vercel preview with no backend): serve canned data.
  if (isDemo) {
    return demoResolve<T>(path, (opts.method ?? "GET").toUpperCase(), opts.body ?? null);
  }
  const res = await fetch(BASE + path, {
    // Same-origin: the httpOnly session cookie rides along automatically.
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(opts.headers || {}),
    },
    ...opts,
  });
  // Session missing/revoked → drop local state and send the user to the login
  // gate. (Full navigation, not router.push, to avoid an import cycle.)
  if (res.status === 401) {
    markLoggedOut();
    if (!window.location.pathname.startsWith("/login")) {
      window.location.assign("/login");
    }
    throw new Error("Signed out — please sign in again.");
  }
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") || "";
  // An HTML body on an /api call means something intercepted the request
  // (transparent ISP cache, captive portal, misrouted proxy). Returning it as
  // data silently corrupts forms that prefill from it — fail loudly instead.
  if (ct.includes("text/html")) {
    throw new Error("Unexpected response from the server — check the connection and retry.");
  }
  if (!ct.includes("application/json")) {
    return (await res.text()) as unknown as T;
  }
  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(p: string) => req<T>(p),
  post: <T>(p: string, body?: unknown) =>
    req<T>(p, { method: "POST", body: JSON.stringify(body ?? {}) }),
  put: <T>(p: string, body?: unknown) =>
    req<T>(p, { method: "PUT", body: JSON.stringify(body ?? {}) }),
  patch: <T>(p: string, body?: unknown) =>
    req<T>(p, { method: "PATCH", body: JSON.stringify(body ?? {}) }),
  del: <T>(p: string) => req<T>(p, { method: "DELETE" }),
};

// Build a querystring from a params object, skipping empty values.
export function qs(params: Record<string, string | number | undefined | null>): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== "") u.set(k, String(v));
  }
  const s = u.toString();
  return s ? `?${s}` : "";
}
