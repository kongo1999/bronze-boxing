// Access-token auth for the deployed app.
//
// The backend enforces a shared bearer token on /api when API_TOKEN is set
// (see backend/internal/server/auth.go). /api/health stays public and reports
// `authRequired`, so the SPA only shows the login gate when auth is actually
// on — local dev and demo mode never see it.

import { isDemo } from "./demo";

const KEY = "bb.token";

export function getToken(): string | null {
  return localStorage.getItem(KEY);
}
export function setToken(token: string): void {
  localStorage.setItem(KEY, token);
}
export function clearToken(): void {
  localStorage.removeItem(KEY);
}

// Whether the API requires a token. Probed once per page load via the public
// /api/health endpoint; fail-open so a flaky probe can't lock the UI out of a
// token-less API (a token-requiring API still 401s and routes to /login).
let required: boolean | null = null;
export async function authRequired(): Promise<boolean> {
  if (isDemo) return false;
  if (required === null) {
    try {
      const res = await fetch("/api/health");
      const j = await res.json();
      required = j?.authRequired === true;
    } catch {
      required = false;
    }
  }
  return required;
}

// Try a candidate token against the protected /api/auth/check endpoint.
export async function verifyToken(token: string): Promise<boolean> {
  try {
    const res = await fetch("/api/auth/check", {
      headers: { Authorization: `Bearer ${token}` },
    });
    return res.ok;
  } catch {
    return false;
  }
}
