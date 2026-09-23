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
	studio = studioInfo{
		Name: defaultStr(cfg.StudioName, "Bronze Boxing Club"), Address: cfg.StudioAddress,
		Phone: cfg.StudioPhone, Currency: defaultStr(cfg.Currency, "$"),
	}
	app := fiber.New(fiber.Config{
		AppName:      "Bronze Boxing API",
		ErrorHandler: errorHandler,
	})

	app.Use(recover.New())
	if !cfg.Quiet {
		app.Use(logger.New(logger.Config{
			Format: "${time} ${status} ${method} ${path} (${latency})\n",
		}))
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))

	// Require a login session when auth is configured (production). CORS
	// preflight (OPTIONS) is already short-circuited above, so this only
	// guards real requests.
	authOn := cfg.AdminPassword != ""
	if authOn {
		app.Use(sameOrigin(cfg.CORSOrigins))
		app.Use(requireSession(store))
	}

	api := app.Group("/api")
	registerHealth(api, store, authOn)
	registerAuth(api, store, authOn)
	registerTrainees(api, store)
	registerSessions(api, store)
	registerPayments(api, store)
	registerReminders(api, store)
	registerExpenses(api, store)
	registerInventory(api, store)
	registerDashboard(api, store)
	registerSearch(api, store)
	registerAudit(api, store)
	registerLedger(api, store)
	registerPlans(api, store)
	registerReports(api, store)

	return app
}
