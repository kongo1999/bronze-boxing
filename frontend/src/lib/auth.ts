// Username/password auth for the deployed app.
//
// The session lives in an httpOnly cookie set by the backend on login — the
// token is never exposed to JS (localStorage/sessionStorage are XSS-readable).
// The browser attaches the cookie automatically on same-origin /api calls, so
// the SPA only tracks *whether* it's signed in, never the token itself.
// /api/health stays public and reports `authRequired`, so the login screen only
// shows when auth is actually on — local dev and demo mode never see it.

import { isDemo } from "./demo";

// Whether the API requires a login. Probed once per page load via the public
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

// Cached sign-in state. Probed once via /api/auth/me (behind the session guard):
// 200 with a username means the cookie is valid, 401 means signed out.
let authed: boolean | null = null;
export async function isAuthed(): Promise<boolean> {
  if (isDemo) return true;
  if (authed === null) {
    try {
      const res = await fetch("/api/auth/me");
      const j = res.ok ? await res.json().catch(() => null) : null;
      authed = !!j?.username;
    } catch {
      authed = false;
    }
  }
  return authed;
}
// Synchronous view of the cached state, for render-time checks (nav, etc.).
export function isLoggedIn(): boolean {
  return authed === true;
}
// The API client calls this when a request comes back 401.
export function markLoggedOut(): void {
  authed = false;
}

// Exchange credentials for a session cookie. `remember` makes it persistent
// (survives a browser restart) vs a session cookie. Returns an error message to
// show, or null on success.
export async function login(username: string, password: string, remember: boolean): Promise<string | null> {
  try {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password, remember }),
    });
    const j = await res.json().catch(() => null);
    if (!res.ok || !j?.token) {
      return j?.error ?? "Couldn't sign in. Try again.";
    }
    authed = true;
    return null;
  } catch {
    return "Couldn't reach the server. Try again.";
  }
}

// Revoke the session server-side (clears the cookie too) and drop local state.
export async function logout(): Promise<void> {
  authed = false;
  try {
    await fetch("/api/auth/logout", { method: "POST" });
  } catch {
    /* session expires on its own */
  }
}
