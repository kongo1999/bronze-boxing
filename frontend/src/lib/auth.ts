// Username/password auth for the deployed app.
//
// The backend keeps user accounts (admin seeded from ADMIN_USERNAME /
// ADMIN_PASSWORD) and issues a session token on POST /api/auth/login; we hold
// it in localStorage and send it as a Bearer header on every call (api.ts).
// /api/health stays public and reports `authRequired`, so the SPA only shows
// the login screen when auth is actually on — local dev and demo mode never
// see it.

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

// Exchange credentials for a session token. Returns an error message to show,
// or null on success (token stored).
export async function login(username: string, password: string): Promise<string | null> {
  try {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    const j = await res.json().catch(() => null);
    if (!res.ok || !j?.token) {
      return j?.error ?? "Couldn't sign in. Try again.";
    }
    setToken(j.token);
    return null;
  } catch {
    return "Couldn't reach the server. Try again.";
  }
}

// Revoke the session server-side (best-effort) and drop the local token.
export async function logout(): Promise<void> {
  const token = getToken();
  clearToken();
  if (token) {
    try {
      await fetch("/api/auth/logout", {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch {
      /* session expires on its own */
    }
  }
}
