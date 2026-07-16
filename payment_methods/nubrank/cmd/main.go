package main

import (
	"context"
	"log/slog"
	"os"

	"nubrank/internal/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main () {
	// Logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config {
		addr: envOr("ADDR", ":8080"),
		db: dbConfig{
			dsn: os.Getenv("DB_DSN"),
		},
	}

	if err := database.Migrate(cfg.db.dsn); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.db.dsn)
	if err != nil {
		slog.Error("failed to create db pool", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	api := application {
		config: cfg,
		db:     db,
	}

	if err := api.run(api.mount()) ; err != nil {
		slog.Error("server has failed to start", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}