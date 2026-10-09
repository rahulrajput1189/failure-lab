package main

import (
	"context"
	"testing"
)

func TestBeginTransaction(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	defer db.Close()

	tx, err := beginTransaction(
		context.Background(),
		db,
	)

	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("failed to rollback transaction: %v", err)
	}
}