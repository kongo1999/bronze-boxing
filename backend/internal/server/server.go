package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
)

// New builds the Fiber app with middleware and registers all /api routes.
func New(cfg config.Config, store *db.Store) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "Bronze Boxing API",
		ErrorHandler: errorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${status} ${method} ${path} (${latency})\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept",
	}))

	api := app.Group("/api")
	registerHealth(api, store)
	registerTrainees(api, store)
	registerSessions(api, store)
	registerPayments(api, store)
	registerReminders(api, store)
	registerExpenses(api, store)
	registerInventory(api, store)
	registerDashboard(api, store)
	registerSearch(api, store)

	return app
}

// errorHandler renders all errors as JSON: { "error": "..." }.
func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
