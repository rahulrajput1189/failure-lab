
package main

import (
	"context"
	"testing"
)

func TestOrderServiceCreateOrder(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)
	inventoryServer := newTestInventoryServer(t)
	service := NewOrderService(db, userRepo, orderRepo, orderItemRepo, inventoryServer.client())

	snapshot := testOrderSnapshot(t, db)
	t.Cleanup(func() {
		cleanupOrdersAfter(t, db, snapshot)
		db.Close()
	})

	order, err := service.CreateOrder(ctx, CreateOrderInput{
		UserID:    1,
		ProductID: 1,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}
	if order.ID == 0 {
		t.Fatal("expected generated order ID")
	}
	if order.UserID != 1 {
		t.Fatalf("expected user ID 1, got %d", order.UserID)
	}
	if order.Status != "PENDING" {
		t.Fatalf("expected status PENDING, got %s", order.Status)
	}
	expectedTotal := 100000 * 2
	if order.TotalCents != expectedTotal {
		t.Fatalf("expected total %d, got %d", expectedTotal, order.TotalCents)
	}

	inventoryProduct, err := inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get inventory product: %v", err)
	}
	if inventoryProduct.Stock != 8 {
		t.Fatalf("expected inventory stock 8, got %d", inventoryProduct.Stock)
	}

	var itemCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items WHERE order_id = $1", order.ID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("failed to verify order item: %v", err)
	}
	if itemCount != 1 {
		t.Fatalf("expected 1 order item, got %d", itemCount)
	}
}

func TestOrderServiceCreateOrderRejectsInvalidUser(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)
	inventoryServer := newTestInventoryServer(t)
	service := NewOrderService(db, userRepo, orderRepo, orderItemRepo, inventoryServer.client())

	t.Cleanup(func() {
		db.Close()
	})

	var initialOrderCount int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&initialOrderCount); err != nil {
		t.Fatalf("failed to count initial orders: %v", err)
	}
	var initialItemCount int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&initialItemCount); err != nil {
		t.Fatalf("failed to count initial order items: %v", err)
	}

	_, err := service.CreateOrder(ctx, CreateOrderInput{
		UserID:    999999,
		ProductID: 1,
		Quantity:  2,
	})
	if err == nil {
		t.Fatal("expected invalid user to be rejected")
	}

	inventoryProduct, err := inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get inventory product: %v", err)
	}
	if inventoryProduct.Stock != 10 {
		t.Fatalf("expected inventory stock to remain 10, got %d", inventoryProduct.Stock)
	}

	var orderCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&orderCount)
	if err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if orderCount != initialOrderCount {
		t.Fatalf("expected order count to remain %d, got %d", initialOrderCount, orderCount)
	}

	var itemCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&itemCount)
	if err != nil {
		t.Fatalf("failed to count order items: %v", err)
	}
	if itemCount != initialItemCount {
		t.Fatalf("expected order item count to remain %d, got %d", initialItemCount, itemCount)
	}
}

func TestOrderServiceCreateOrderRejectsInvalidProduct(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)
	inventoryServer := newTestInventoryServer(t)
	service := NewOrderService(db, userRepo, orderRepo, orderItemRepo, inventoryServer.client())

	t.Cleanup(func() {
		db.Close()
	})

	var initialOrderCount int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&initialOrderCount); err != nil {
		t.Fatalf("failed to count initial orders: %v", err)
	}
	var initialItemCount int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&initialItemCount); err != nil {
		t.Fatalf("failed to count initial order items: %v", err)
	}

	_, err := service.CreateOrder(ctx, CreateOrderInput{
		UserID:    1,
		ProductID: 999999,
		Quantity:  2,
	})
	if err == nil {
		t.Fatal("expected invalid product to be rejected")
	}

	inventoryProduct, err := inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get inventory product: %v", err)
	}
	if inventoryProduct.Stock != 10 {
		t.Fatalf("expected inventory stock to remain 10, got %d", inventoryProduct.Stock)
	}

	var orderCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&orderCount)
	if err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if orderCount != initialOrderCount {
		t.Fatalf("expected order count to remain %d, got %d", initialOrderCount, orderCount)
	}

	var itemCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&itemCount)
	if err != nil {
		t.Fatalf("failed to count order items: %v", err)
	}
	if itemCount != initialItemCount {
		t.Fatalf("expected order item count to remain %d, got %d", initialItemCount, itemCount)
	}
}

func TestOrderServiceCreateOrderRejectsInsufficientStock(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)
	inventoryServer := newTestInventoryServer(t)
	service := NewOrderService(db, userRepo, orderRepo, orderItemRepo, inventoryServer.client())

	t.Cleanup(func() {
		db.Close()
	})

	var initialOrderCount int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&initialOrderCount)
	if err != nil {
		t.Fatalf("failed to count initial orders: %v", err)
	}
	var initialItemCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&initialItemCount)
	if err != nil {
		t.Fatalf("failed to count initial order items: %v", err)
	}

	_, err = service.CreateOrder(ctx, CreateOrderInput{
		UserID:    1,
		ProductID: 1,
		Quantity:  11,
	})
	if err == nil {
		t.Fatal("expected insufficient stock to be rejected")
	}

	inventoryProduct, err := inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get inventory product: %v", err)
	}
	if inventoryProduct.Stock != 10 {
		t.Fatalf("expected inventory stock to remain 10, got %d", inventoryProduct.Stock)
	}

	var orderCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&orderCount)
	if err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if orderCount != initialOrderCount {
		t.Fatalf("expected order count to remain %d, got %d", initialOrderCount, orderCount)
	}

	var itemCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&itemCount)
	if err != nil {
		t.Fatalf("failed to count order items: %v", err)
	}
	if itemCount != initialItemCount {
		t.Fatalf("expected order item count to remain %d, got %d", initialItemCount, itemCount)
	}
}

func TestOrderServiceCreateOrderConcurrent(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)
	inventoryServer := newTestInventoryServer(t)
	service := NewOrderService(db, userRepo, orderRepo, orderItemRepo, inventoryServer.client())

	snapshot := testOrderSnapshot(t, db)
	t.Cleanup(func() {
		cleanupOrdersAfter(t, db, snapshot)
		db.Close()
	})

	var initialOrderCount int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&initialOrderCount); err != nil {
		t.Fatalf("failed to count initial orders: %v", err)
	}
	var initialItemCount int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&initialItemCount); err != nil {
		t.Fatalf("failed to count initial order items: %v", err)
	}

	const attempts = 20
	results := make(chan error, attempts)

	for i := 0; i < attempts; i++ {
		go func() {
			_, err := service.CreateOrder(ctx, CreateOrderInput{
				UserID:    1,
				ProductID: 1,
				Quantity:  1,
			})
			results <- err
		}()
	}

	successCount := 0
	for i := 0; i < attempts; i++ {
		if err := <-results; err == nil {
			successCount++
		}
	}
	if successCount != 10 {
		t.Fatalf("expected 10 successful orders, got %d", successCount)
	}

	inventoryProduct, err := inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get inventory product: %v", err)
	}
	if inventoryProduct.Stock != 0 {
		t.Fatalf("expected inventory stock 0, got %d", inventoryProduct.Stock)
	}

	var orderCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM orders").Scan(&orderCount)
	if err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}
	if orderCount != initialOrderCount+10 {
		t.Fatalf("expected order count %d, got %d", initialOrderCount+10, orderCount)
	}

	var itemCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM order_items").Scan(&itemCount)
	if err != nil {
		t.Fatalf("failed to count order items: %v", err)
	}
	if itemCount != initialItemCount+10 {
		t.Fatalf("expected order item count %d, got %d", initialItemCount+10, itemCount)
	}
}

func TestOrderServiceGetOrder(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)
	orderService := NewOrderService(db, userRepo, orderRepo, orderItemRepo)

	snapshot := testOrderSnapshot(t, db)
	t.Cleanup(func() {
		cleanupOrdersAfter(t, db, snapshot)
		db.Close()
	})

	order, err := orderRepo.Create(ctx, 1, "PENDING", 100000)
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	found, err := orderService.GetOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if found.ID != order.ID {
		t.Fatalf("expected order ID %d, got %d", order.ID, found.ID)
	}
	if found.UserID != 1 {
		t.Fatalf("expected user ID 1, got %d", found.UserID)
	}
	if found.Status != "PENDING" {
		t.Fatalf("expected status PENDING, got %s", found.Status)
	}
	if found.TotalCents != 100000 {
		t.Fatalf("expected total 100000, got %d", found.TotalCents)
	}
}
