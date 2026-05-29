package server

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"bronzeboxing/internal/db"
)

// registerHealth exposes GET /api/health to diagnose DB connectivity.
func registerHealth(r fiber.Router, store *db.Store) {
	r.Get("/health", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		dbOK := store.Ping(ctx) == nil
		status := "ok"
		if !dbOK {
			status = "degraded"
		}
		return c.JSON(fiber.Map{
			"status": status,
			"db":     dbOK,
			"time":   time.Now().UTC(),
		})
	})
}
