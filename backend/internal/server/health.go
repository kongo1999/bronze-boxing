package server

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"bronzeboxing/internal/db"
)

// registerHealth exposes GET /api/health to diagnose DB connectivity. It also
// tells the SPA whether the API requires a login (authRequired), so the
// sign-in screen only appears when ADMIN_PASSWORD is configured. This endpoint
// stays public so uptime checks work without credentials.
func registerHealth(r fiber.Router, store *db.Store, authRequired bool) {
	r.Get("/health", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		dbOK := store.Ping(ctx) == nil
		status := "ok"
		if !dbOK {
			status = "degraded"
		}
		return c.JSON(fiber.Map{
			"status":       status,
			"db":           dbOK,
			"authRequired": authRequired,
			"time":         time.Now().UTC(),
		})
	})
}
