package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
	"bronzeboxing/internal/server"
)

func main() {
	// Load .env if present (optional; real env vars win).
	_ = godotenv.Load()

	cfg := config.Load()

	store, err := db.Connect(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatalf("mongo connect failed: %v", err)
	}
	log.Printf("connected to MongoDB (%s/%s)", cfg.MongoURI, cfg.DBName)

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
