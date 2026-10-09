package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newDatabasePool(config Config) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(config.databaseURL())
	if err != nil {
		log.Fatalf("failed to parse database configuration: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalf("failed to create database pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		log.Fatalf("failed to connect to database: %v", err)
	}

	log.Println("connected to PostgreSQL")

	return pool
}

func beginTransaction(
	ctx context.Context,
	db *pgxpool.Pool,
) (pgx.Tx, error) {
	return db.Begin(ctx)
}