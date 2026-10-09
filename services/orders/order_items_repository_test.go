package main

import (
	"context"
	"testing"
)

func TestOrderItemRepositoryCreate(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	repo := NewOrderRepository(db)
	itemRepo := NewOrderItemRepository(db)

	order, err := repo.Create(
		context.Background(),
		1,
		"PENDING",
		100000,
	)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create test order: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			"DELETE FROM order_items WHERE order_id = $1",
			order.ID,
		)
		if err != nil {
			t.Errorf("failed to clean up test order item: %v", err)
		}
	
		_, err = db.Exec(
			context.Background(),
			"DELETE FROM orders WHERE id = $1",
			order.ID,
		)
		if err != nil {
			t.Errorf("failed to clean up test order: %v", err)
		}
	
		db.Close()
	}()

	item, err := itemRepo.Create(
		context.Background(),
		order.ID,
		1,
		1,
		100000,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if item.ID == 0 {
		t.Fatal("expected generated order item ID")
	}

	if item.OrderID != order.ID {
		t.Fatalf(
			"expected order ID %d, got %d",
			order.ID,
			item.OrderID,
		)
	}

	if item.ProductID != 1 {
		t.Fatalf(
			"expected product ID 1, got %d",
			item.ProductID,
		)
	}

	if item.Quantity != 1 {
		t.Fatalf(
			"expected quantity 1, got %d",
			item.Quantity,
		)
	}

	if item.PriceCents != 100000 {
		t.Fatalf(
			"expected price 100000, got %d",
			item.PriceCents,
		)
	}
}