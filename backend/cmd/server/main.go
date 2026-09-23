package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // embed zoneinfo so STUDIO_TZ resolves inside distroless

	"github.com/joho/godotenv"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
	"bronzeboxing/internal/migrate"
	"bronzeboxing/internal/server"
)

func main() {
	// Load .env if present (optional; real env vars win).
	_ = godotenv.Load()

	cfg := config.Load()

	// Pin the process to the studio's timezone: every time.Local computation
	// (month ranges, "today" windows, statements) follows the gym's clock,
	// not the container's UTC.
	if loc, err := time.LoadLocation(cfg.Timezone); err == nil {
		time.Local = loc
		log.Printf("studio timezone: %s", cfg.Timezone)
	} else {
		log.Printf("WARNING: invalid STUDIO_TZ %q (%v) — falling back to server local time", cfg.Timezone, err)
	}

	store, err := db.Connect(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatalf("mongo connect failed: %v", err)
	}
	log.Printf("connected to MongoDB (db %s, transactions %v)", cfg.DBName, store.SupportsTx)

	// Money and stock changes are multi-document transactions; a standalone
	// mongod can't run them. Refuse to start rather than write half-changes.
	if !store.SupportsTx {
		if !cfg.AllowStandalone {
			log.Fatal("MongoDB is not a replica set, so transactions are unavailable. " +
				"Run it as a single-node replica set (see DEPLOY.md / README.md), " +
				"or set MONGO_ALLOW_STANDALONE=true to run degraded in an emergency.")
		}
		db.AllowDegraded = true
		log.Println("WARNING: MONGO_ALLOW_STANDALONE=true — money/stock changes are NOT atomic")
	}

	// Schema/data migrations run explicitly (cmd/migrate), never implicitly.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pending, err := migrate.Pending(ctx, store, server.Migrations())
		cancel()
		if err != nil {
			log.Fatalf("migration check failed: %v", err)
		}
		if len(pending) > 0 {
			ids := make([]string, len(pending))
			for i, m := range pending {
				ids[i] = m.ID
			}
			msg := "pending migrations: " + strings.Join(ids, ", ") +
				" — back up, then run `go run ./cmd/migrate` (or `docker compose run --rm migrate`)"
			if !cfg.SkipMigrationCheck {
				log.Fatal(msg)
			}
			log.Println("WARNING: " + msg)
		}
	}

	// Auth indexes + admin account bootstrap (synced from ADMIN_USERNAME /
	// ADMIN_PASSWORD; password rotation revokes that account's sessions).
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := server.EnsureAuth(ctx, store, cfg.AdminUsername, cfg.AdminPassword); err != nil {
			cancel()
			log.Fatalf("auth bootstrap failed: %v", err)
		}
		cancel()
		if cfg.AdminPassword != "" {
			log.Printf("auth enabled — admin account %q ready", cfg.AdminUsername)
		} else {
			log.Println("auth DISABLED (no ADMIN_PASSWORD set) — do not expose this to the internet")
		}
	}

	app := server.New(cfg, store)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatalf("server listen failed: %v", err)
		}
	}()
	log.Printf("Bronze Boxing API listening on :%s", cfg.Port)

	// Graceful shutdown on Ctrl-C / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(ctx)
	_ = store.Disconnect(ctx)
	log.Println("bye")
}
