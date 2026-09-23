package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bronzeboxing/internal/config"
)

// With auth on: login answers without the token (it lives only in the
// httpOnly cookie), the cookie is Secure behind HTTPS, a state-changing
// request from another site is refused, and logout ends the session.
func TestLoginKeepsTheTokenOutOfJavaScript(t *testing.T) {
	e := newEnv(t)
	if err := EnsureAuth(e.ctx, e.store, "admin", "s3cret-pass"); err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{CORSOrigins: "*", Quiet: true, AdminUsername: "admin", AdminPassword: "s3cret-pass"}, e.store)
	do := func(method, path, body, cookie, origin string) (*http.Response, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = "studio.example"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-Proto", "https")
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		return res, raw
	}

	res, raw := do("POST", "/api/auth/login", `{"username":"admin","password":"s3cret-pass"}`, "", "https://studio.example")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d %s", res.StatusCode, raw)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if _, leaked := body["token"]; leaked || bytes.Contains(raw, []byte("passwordHash")) {
		t.Fatalf("login JSON exposes secrets: %s", raw)
	}
	if body["username"] != "admin" {
		t.Fatalf("login JSON = %s", raw)
	}
	set := res.Header.Get("Set-Cookie")
	if !strings.Contains(set, "HttpOnly") || !strings.Contains(set, "secure") || !strings.Contains(strings.ToLower(set), "samesite=lax") {
		t.Fatalf("cookie attributes: %s", set)
	}
	cookie := strings.SplitN(set, ";", 2)[0]

	if res, _ := do("GET", "/api/trainees", "", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", res.StatusCode)
	}
	if res, raw := do("POST", "/api/expenses", `{"amount":5,"category":"other"}`, cookie, "https://evil.example"); res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site POST: %d %s", res.StatusCode, raw)
	}
	if res, raw := do("POST", "/api/expenses", `{"amount":5,"category":"other"}`, cookie, "https://studio.example"); res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("same-site POST: %d %s", res.StatusCode, raw)
	}

	res, _ = do("POST", "/api/auth/logout", "", cookie, "https://studio.example")
	if exp := res.Header.Get("Set-Cookie"); !strings.Contains(exp, "Max-Age=0") && !strings.Contains(exp, "expires=") {
		t.Fatalf("logout cookie not expired: %s", exp)
	}
	if res, _ := do("GET", "/api/trainees", "", cookie, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout the old cookie still works: %d", res.StatusCode)
	}
}
