package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestCancelOrderReleaseFailureCanRetryWithoutDoubleRelease(t *testing.T) {
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
		t.Fatalf("create order: %v", err)
	}

	var paymentMu sync.Mutex
	paymentStatus := PaymentStatusAuthorized
	paymentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paymentMu.Lock()
		defer paymentMu.Unlock()

		if r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/payments/order/%d", order.ID) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Payment{ID: 700, OrderID: order.ID, Status: paymentStatus, AmountCents: order.TotalCents})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/payments/700/refund" {
			if paymentStatus == PaymentStatusRefunded {
				http.Error(w, "invalid payment state transition", http.StatusConflict)
				return
			}
			if paymentStatus != PaymentStatusAuthorized {
				http.Error(w, "payment is not authorized", http.StatusConflict)
				return
			}
			paymentStatus = PaymentStatusRefunded
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Payment{ID: 700, OrderID: order.ID, Status: paymentStatus, AmountCents: order.TotalCents})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(paymentServer.Close)
	service.SetPaymentClient(NewPaymentClient(paymentServer.URL, paymentServer.Client()))

	service.failAfterInventoryRelease = true
	if _, err := service.CancelOrder(ctx, order.ID); err == nil {
		t.Fatal("expected injected failure after inventory release")
	}

	pendingRecovery, err := orderRepo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("get order after injected failure: %v", err)
	}
	if pendingRecovery.Status != "CANCELLING" {
		t.Fatalf("expected order status CANCELLING, got %s", pendingRecovery.Status)
	}
	paymentMu.Lock()
	if paymentStatus != PaymentStatusRefunded {
		t.Fatalf("expected payment to be refunded before failure, got %s", paymentStatus)
	}
	paymentMu.Unlock()

	inventoryProduct, err := inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("get inventory after first cancellation attempt: %v", err)
	}
	if inventoryProduct.Stock != 10 {
		t.Fatalf("expected stock 10 after release, got %d", inventoryProduct.Stock)
	}

	service.failAfterInventoryRelease = false
	cancelled, err := service.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("retry cancellation: %v", err)
	}
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("expected CANCELLED, got %s", cancelled.Status)
	}

	inventoryProduct, err = inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("get inventory after retry: %v", err)
	}
	if inventoryProduct.Stock != 10 {
		t.Fatalf("expected stock to remain 10 after retry, got %d", inventoryProduct.Stock)
	}

	// A duplicate cancellation request must not refund or release twice.
	if _, err := service.CancelOrder(ctx, order.ID); err != nil {
		t.Fatalf("repeat cancellation: %v", err)
	}
	inventoryProduct, err = inventoryServer.client().GetProduct(ctx, 1)
	if err != nil {
		t.Fatalf("get inventory after duplicate cancellation: %v", err)
	}
	if inventoryProduct.Stock != 10 {
		t.Fatalf("expected duplicate cancellation not to change stock, got %d", inventoryProduct.Stock)
	}
	paymentMu.Lock()
	defer paymentMu.Unlock()
	if paymentStatus != PaymentStatusRefunded {
		t.Fatalf("expected payment to remain refunded, got %s", paymentStatus)
	}
}

func TestCancelConfirmedOrder(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)
	ctx := context.Background()
	orderRepo := NewOrderRepository(db)
	t.Cleanup(func() { db.Close() })

	order, err := orderRepo.Create(ctx, 1, "CONFIRMED", 2500)
	if err != nil {
		t.Fatalf("create confirmed order: %v", err)
	}
	t.Cleanup(func() {
		_, err := db.Exec(ctx, "DELETE FROM orders WHERE id = $1", order.ID)
		if err != nil {
			t.Errorf("cleanup confirmed order: %v", err)
		}
	})

	inventoryServer := newTestInventoryServer(t)
	service := NewOrderService(db, NewUserRepository(db), orderRepo, NewOrderItemRepository(db), inventoryServer.client())
	if _, err := inventoryServer.client().ReserveStock(ctx, 1, 1); err != nil {
		t.Fatalf("reserve stock for confirmed order: %v", err)
	}
	itemRepo := NewOrderItemRepository(db)
	if _, err := itemRepo.Create(ctx, order.ID, 1, 1, 2500); err != nil {
		t.Fatalf("create order item: %v", err)
	}
	t.Cleanup(func() {
		_, err := db.Exec(ctx, "DELETE FROM order_items WHERE order_id = $1", order.ID)
		if err != nil {
			t.Errorf("cleanup order item: %v", err)
		}
	})

	cancelled, err := service.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("cancel confirmed order: %v", err)
	}
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("expected CANCELLED, got %s", cancelled.Status)
	}
}
