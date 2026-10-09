package main

import (
	"context"
	"testing"
)

func TestUserRepositoryExists(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	defer db.Close()

	repo := NewUserRepository(db)

	exists, err := repo.Exists(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !exists {
		t.Fatal("expected user 1 to exist")
	}

	exists, err = repo.Exists(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exists {
		t.Fatal("expected user 999 not to exist")
	}
}