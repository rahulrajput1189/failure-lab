package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateOrderInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/orders",
		nil,
	)

	rec := httptest.NewRecorder()

	createOrder(rec, req, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}

	if rec.Body.String() != "invalid JSON\n" {
		t.Fatalf(
			"expected invalid JSON error, got %q",
			rec.Body.String(),
		)
	}
}

func TestCreateOrderSuccess(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	ctx := context.Background()

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	inventoryServer := newTestInventoryServer(t)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
		inventoryServer.client(),
	)

	t.Cleanup(func() {
		_, err := db.Exec(
			ctx,
			"DELETE FROM order_items",
		)
		if err != nil {
			t.Errorf("failed to clean order items: %v", err)
		}

		_, err = db.Exec(
			ctx,
			"DELETE FROM orders",
		)
		if err != nil {
			t.Errorf("failed to clean orders: %v", err)
		}

		db.Close()
	})

	body := strings.NewReader(
		`{"user_id":1,"product_id":1,"quantity":2}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/orders",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	createOrder(rec, req, orderService)

	if rec.Code != http.StatusCreated {
		t.Fatalf(
			"expected status %d, got %d: %s",
			http.StatusCreated,
			rec.Code,
			rec.Body.String(),
		)
	}

	var order Order

	if err := json.NewDecoder(rec.Body).Decode(&order); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if order.UserID != 1 {
		t.Fatalf(
			"expected user ID 1, got %d",
			order.UserID,
		)
	}

	if order.Status != "PENDING" {
		t.Fatalf(
			"expected status PENDING, got %s",
			order.Status,
		)
	}

	if order.TotalCents != 200000 {
		t.Fatalf(
			"expected total 200000, got %d",
			order.TotalCents,
		)
	}

	inventoryServer.mu.Lock()
	remainingStock := inventoryServer.stock[1]
	inventoryServer.mu.Unlock()

	if remainingStock != 8 {
		t.Fatalf(
			"expected inventory stock 8, got %d",
			remainingStock,
		)
	}
}

func TestCreateOrderInvalidUser(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	ctx := context.Background()

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	inventoryServer := newTestInventoryServer(t)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
		inventoryServer.client(),
	)

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

	body := strings.NewReader(
		`{"user_id":999999,"product_id":1,"quantity":1}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/orders",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	createOrder(rec, req, orderService)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}

	if rec.Body.String() != "invalid user: user 999999 does not exist\n" {
		t.Fatalf(
			"unexpected response: %q",
			rec.Body.String(),
		)
	}

	inventoryServer.mu.Lock()
	remainingStock := inventoryServer.stock[1]
	inventoryServer.mu.Unlock()

	if remainingStock != 10 {
		t.Fatalf(
			"expected inventory stock 10, got %d",
			remainingStock,
		)
	}

	var orderCount int

	err := db.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM orders",
	).Scan(&orderCount)

	if err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}

	if orderCount != 0 {
		t.Fatalf(
			"expected 0 orders, got %d",
			orderCount,
		)
	}
}

func TestCreateOrderInvalidProduct(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	ctx := context.Background()

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	inventoryServer := newTestInventoryServer(t)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
		inventoryServer.client(),
	)

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

	body := strings.NewReader(
		`{"user_id":1,"product_id":999999,"quantity":1}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/orders",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	createOrder(rec, req, orderService)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}

	if rec.Body.String() != "invalid product: product 999999 does not exist\n" {
		t.Fatalf(
			"unexpected response: %q",
			rec.Body.String(),
		)
	}

	inventoryServer.mu.Lock()
	remainingStock := inventoryServer.stock[1]
	inventoryServer.mu.Unlock()

	if remainingStock != 10 {
		t.Fatalf(
			"expected inventory stock 10, got %d",
			remainingStock,
		)
	}
}

func TestCreateOrderInsufficientStock(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	ctx := context.Background()

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	inventoryServer := newTestInventoryServer(t)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
		inventoryServer.client(),
	)

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

	originalStock := 10

	body := strings.NewReader(
		fmt.Sprintf(
			`{"user_id":1,"product_id":1,"quantity":%d}`,
			originalStock+1,
		),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/orders",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	createOrder(rec, req, orderService)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}

	expected := fmt.Sprintf(
		"insufficient stock: product %d: requested %d, available %d\n",
		1,
		originalStock+1,
		originalStock,
	)

	if rec.Body.String() != expected {
		t.Fatalf(
			"unexpected response: %q",
			rec.Body.String(),
		)
	}

	inventoryServer.mu.Lock()
	remainingStock := inventoryServer.stock[1]
	inventoryServer.mu.Unlock()

	if remainingStock != originalStock {
		t.Fatalf(
			"expected inventory stock %d, got %d",
			originalStock,
			remainingStock,
		)
	}

	var orderCount int

	err := db.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM orders",
	).Scan(&orderCount)

	if err != nil {
		t.Fatalf("failed to count orders: %v", err)
	}

	if orderCount != 0 {
		t.Fatalf(
			"expected 0 orders, got %d",
			orderCount,
		)
	}
}

func TestGetOrderSuccess(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	ctx := context.Background()

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
	)

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

	order, err := orderRepo.Create(
		ctx,
		1,
		"PENDING",
		100000,
	)

	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/orders/%d", order.ID),
		nil,
	)

	rec := httptest.NewRecorder()

	getOrder(rec, req, orderService)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			rec.Code,
		)
	}

	var response Order

	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.ID != order.ID {
		t.Fatalf(
			"expected order ID %d, got %d",
			order.ID,
			response.ID,
		)
	}

	if response.UserID != 1 {
		t.Fatalf(
			"expected user ID 1, got %d",
			response.UserID,
		)
	}

	if response.Status != "PENDING" {
		t.Fatalf(
			"expected status PENDING, got %s",
			response.Status,
		)
	}

	if response.TotalCents != 100000 {
		t.Fatalf(
			"expected total 100000, got %d",
			response.TotalCents,
		)
	}
}

func TestGetOrderNotFound(t *testing.T) {
	config := loadConfig()
	db := newDatabasePool(config)

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
	)

	t.Cleanup(func() {
		db.Close()
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/orders/999999",
		nil,
	)

	rec := httptest.NewRecorder()

	getOrder(rec, req, orderService)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusNotFound,
			rec.Code,
		)
	}

	if rec.Body.String() != "order not found\n" {
		t.Fatalf(
			"unexpected response: %q",
			rec.Body.String(),
		)
	}
}

func TestGetOrderInvalidID(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodGet,
		"/orders/abc",
		nil,
	)

	rec := httptest.NewRecorder()

	getOrder(rec, req, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}

	if rec.Body.String() != "invalid order ID\n" {
		t.Fatalf(
			"unexpected response: %q",
			rec.Body.String(),
		)
	}
}
