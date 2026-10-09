package main

import (
	"context"
	"testing"
)

func TestCreateOrderWithItemInTransaction(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	defer db.Close()

	ctx := context.Background()

	tx, err := beginTransaction(ctx, db)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	orderRepo := NewOrderRepository(tx)
	itemRepo := NewOrderItemRepository(tx)

	order, err := orderRepo.Create(
		ctx,
		1,
		"PENDING",
		100000,
	)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("failed to create order: %v", err)
	}

	item, err := itemRepo.Create(
		ctx,
		order.ID,
		1,
		1,
		100000,
	)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("failed to create order item: %v", err)
	}

	if item.OrderID != order.ID {
		tx.Rollback(ctx)
		t.Fatalf(
			"expected item order ID %d, got %d",
			order.ID,
			item.OrderID,
		)
	}

	var orderCount int

	err = db.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM orders WHERE id = $1",
		order.ID,
	).Scan(&orderCount)

	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("failed to check order: %v", err)
	}

	if orderCount != 0 {
		tx.Rollback(ctx)
		t.Fatal("order should not be visible outside transaction")
	}

	var itemCount int

	err = db.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM order_items WHERE id = $1",
		item.ID,
	).Scan(&itemCount)

	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("failed to check order item: %v", err)
	}

	if itemCount != 0 {
		tx.Rollback(ctx)
		t.Fatal("order item should not be visible outside transaction")
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to rollback transaction: %v", err)
	}
}

func TestOrderTransactionRollsBackWhenItemCreationFails(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	defer db.Close()

	ctx := context.Background()

	tx, err := beginTransaction(ctx, db)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	orderRepo := NewOrderRepository(tx)
	itemRepo := NewOrderItemRepository(tx)

	order, err := orderRepo.Create(
		ctx,
		1,
		"PENDING",
		100000,
	)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("failed to create order: %v", err)
	}

	_, err = itemRepo.Create(
		ctx,
		order.ID,
		999,
		1,
		100000,
	)

	if err == nil {
		tx.Rollback(ctx)
		t.Fatal("expected order item creation to fail")
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("failed to rollback transaction: %v", err)
	}

	var orderCount int

	err = db.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM orders WHERE id = $1",
		order.ID,
	).Scan(&orderCount)

	if err != nil {
		t.Fatalf("failed to verify rollback: %v", err)
	}

	if orderCount != 0 {
		t.Fatalf(
			"expected order to be rolled back, but found %d order(s)",
			orderCount,
		)
	}
}