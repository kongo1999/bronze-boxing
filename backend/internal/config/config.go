package config

import "os"

// Config holds runtime configuration, sourced from env vars with sane defaults
// for local development on this machine (native MongoDB on 27017, no Docker).
type Config struct {
	MongoURI      string
	DBName        string
	Port          string
	CORSOrigins   string
	Currency      string
	AdminUsername string
	AdminPassword string
}

func Load() Config {
	return Config{
		MongoURI:    env("MONGODB_URI", "mongodb://127.0.0.1:27017"),
		DBName:      env("DB_NAME", "bronze-boxing"),
		Port:        env("PORT", "8080"),
		CORSOrigins: env("CORS_ORIGINS", "http://localhost:5173"),
		Currency:    env("CURRENCY", "$"),
		// Login auth. Empty password = open mode (local dev). Set in
		// production: the admin account is created/synced from these on boot.
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", ""),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
