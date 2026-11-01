package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newDatabasePool(config Config) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(config.databaseURL())
	if err != nil {
		slog.Error("failed to parse database configuration",
			"service", "inventory",
			"error", err,
		)
		os.Exit(1)
		return nil
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		slog.Error("failed to create database pool",
			"service", "inventory",
			"error", err,
		)
		os.Exit(1)
		return nil
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		slog.Error("failed to connect to database",
			"service", "inventory",
			"error", err,
		)
		os.Exit(1)
		return nil
	}

	slog.Info("connected to PostgreSQL",
		"service", "inventory",
	)

	return pool
}

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
}
