// Package config resolves runtime configuration from environment variables
// (with flag overrides), so the server works with zero configuration
// (defaults to a local SQLite file) and can be pointed at an existing MySQL
// instance for a real deployment without any code changes.
package config

import (
	"flag"
	"os"
)

type Config struct {
	Port      string
	DBDriver  string // "sqlite" (default) or "mysql"
	DBDSN     string
	AdminUser string // ADMIN_USER; if set with AdminPass, upserted as an admin account on every boot
	AdminPass string // ADMIN_PASS
}

func Load() Config {
	cfg := Config{
		Port:      envOr("PORT", "8080"),
		DBDriver:  envOr("DB_DRIVER", "sqlite"),
		DBDSN:     envOr("DB_DSN", "./data/ocpp-simulator.db"),
		AdminUser: envOr("ADMIN_USER", ""),
		AdminPass: envOr("ADMIN_PASS", ""),
	}
	flag.StringVar(&cfg.Port, "port", cfg.Port, "HTTP port to listen on")
	flag.StringVar(&cfg.DBDriver, "db-driver", cfg.DBDriver, "database driver: sqlite or mysql")
	flag.StringVar(&cfg.DBDSN, "db-dsn", cfg.DBDSN, "sqlite file path, or MySQL DSN when db-driver=mysql")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
