package server

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// requireToken enforces a shared bearer token on all /api routes when API_TOKEN
// is configured. /api/health stays public so uptime / load-balancer checks work
// without the secret. Comparison is constant-time to avoid timing leaks.
func requireToken(token string) fiber.Handler {
	want := []byte(token)
	return func(c *fiber.Ctx) error {
		if c.Path() == "/api/health" {
			return c.Next()
		}
		got := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		return c.Next()
	}
}
