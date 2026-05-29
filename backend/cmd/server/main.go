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
