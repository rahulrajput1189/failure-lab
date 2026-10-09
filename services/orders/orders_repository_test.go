package main

import (
	"context"
	"testing"
)

func TestOrderRepositoryCreate(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	repo := NewOrderRepository(db)

	order, err := repo.Create(
		context.Background(),
		1,
		"PENDING",
		100000,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			"DELETE FROM orders WHERE id = $1",
			order.ID,
		)

		if err != nil {
			t.Errorf("failed to clean up test order: %v", err)
		}
		defer db.Close()
	})

	if order.ID == 0 {
		t.Fatal("expected generated order ID")
	}

	if order.UserID != 1 {
		t.Fatalf("expected user ID 1, got %d", order.UserID)
	}

	if order.Status != "PENDING" {
		t.Fatalf("expected status PENDING, got %s", order.Status)
	}

	if order.TotalCents != 100000 {
		t.Fatalf("expected total 100000, got %d", order.TotalCents)
	}
}

func TestOrderRepositoryGetByID(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	ctx := context.Background()

	t.Cleanup(func() {
		_, err := db.Exec(ctx, "DELETE FROM order_items")
		if err != nil {
			t.Errorf("failed to clean order items: %v", err)
		}

		_, err = db.Exec(ctx, "DELETE FROM orders")
		if err != nil {
			t.Errorf("failed to clean orders: %v", err)
		}

		db.Close()
	})

	repo := NewOrderRepository(db)

	order, err := repo.Create(
		ctx,
		1,
		"PENDING",
		100000,
	)

	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	found, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}

	if found.ID != order.ID {
		t.Fatalf(
			"expected order ID %d, got %d",
			order.ID,
			found.ID,
		)
	}

	if found.UserID != 1 {
		t.Fatalf(
			"expected user ID 1, got %d",
			found.UserID,
		)
	}

	if found.Status != "PENDING" {
		t.Fatalf(
			"expected status PENDING, got %s",
			found.Status,
		)
	}

	if found.TotalCents != 100000 {
		t.Fatalf(
			"expected total 100000, got %d",
			found.TotalCents,
		)
	}
}
