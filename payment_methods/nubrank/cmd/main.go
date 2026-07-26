package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"nubrank/internal/chaos"
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
		chaos: chaos.Config{
			LatencyMin:     time.Duration(envOrInt("CHAOS_LATENCY_MIN_MS", 0)) * time.Millisecond,
			LatencyMax:     time.Duration(envOrInt("CHAOS_LATENCY_MAX_MS", 0)) * time.Millisecond,
			ErrorRate:      envOrFloat("CHAOS_ERROR_RATE", 0),
			RateLimitRPS:   envOrFloat("CHAOS_RATE_LIMIT_RPS", 0),
			RateLimitBurst: envOrInt("CHAOS_RATE_LIMIT_BURST", 0),
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

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid int env var, using fallback", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return n
}

func envOrFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		slog.Warn("invalid float env var, using fallback", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return f
}