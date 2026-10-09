package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newInventoryHTTPTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	config := loadConfig()
	db := newDatabasePool(config)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func resetInventoryHTTPTestProduct(
	t *testing.T,
	db *pgxpool.Pool,
) {
	t.Helper()

	_, err := db.Exec(
		context.Background(),
		`UPDATE products
		 SET stock = 10
		 WHERE id = 1`,
	)

	if err != nil {
		t.Fatalf("failed to reset product stock: %v", err)
	}
}

func TestGetProductSuccess(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	resetInventoryHTTPTestProduct(t, db)

	repo := NewProductRepository(db)

	req := httptest.NewRequest(
		http.MethodGet,
		"/products/1",
		nil,
	)

	rec := httptest.NewRecorder()

	getProduct(rec, req, repo)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			rec.Code,
		)
	}

	var product Product

	if err := json.NewDecoder(rec.Body).Decode(&product); err != nil {
		t.Fatalf(
			"failed to decode response: %v",
			err,
		)
	}

	if product.ID != 1 {
		t.Fatalf(
			"expected product ID 1, got %d",
			product.ID,
		)
	}

	if product.Stock != 10 {
		t.Fatalf(
			"expected stock 10, got %d",
			product.Stock,
		)
	}
}

func TestGetProductNotFound(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	repo := NewProductRepository(db)

	req := httptest.NewRequest(
		http.MethodGet,
		"/products/999999",
		nil,
	)

	rec := httptest.NewRecorder()

	getProduct(rec, req, repo)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"expected status 404, got %d",
			rec.Code,
		)
	}
}

func TestGetProductInvalidID(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	repo := NewProductRepository(db)

	req := httptest.NewRequest(
		http.MethodGet,
		"/products/abc",
		nil,
	)

	rec := httptest.NewRecorder()

	getProduct(rec, req, repo)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status 400, got %d",
			rec.Code,
		)
	}
}

func TestReserveStockSuccess(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	resetInventoryHTTPTestProduct(t, db)

	repo := NewProductRepository(db)

	body := bytes.NewBufferString(
		`{"quantity":3}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/products/1/reserve",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	reserveStock(rec, req, repo)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			rec.Code,
		)
	}

	var product Product

	if err := json.NewDecoder(rec.Body).Decode(&product); err != nil {
		t.Fatalf(
			"failed to decode response: %v",
			err,
		)
	}

	if product.Stock != 7 {
		t.Fatalf(
			"expected stock 7, got %d",
			product.Stock,
		)
	}

	resetInventoryHTTPTestProduct(t, db)
}

func TestReserveStockInsufficientStock(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	resetInventoryHTTPTestProduct(t, db)

	repo := NewProductRepository(db)

	body := bytes.NewBufferString(
		`{"quantity":11}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/products/1/reserve",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	reserveStock(rec, req, repo)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status 400, got %d",
			rec.Code,
		)
	}

	resetInventoryHTTPTestProduct(t, db)
}

func TestReserveStockInvalidQuantity(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	repo := NewProductRepository(db)

	body := bytes.NewBufferString(
		`{"quantity":0}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/products/1/reserve",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	reserveStock(rec, req, repo)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status 400, got %d",
			rec.Code,
		)
	}
}

func TestReleaseStockSuccess(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	resetInventoryHTTPTestProduct(t, db)

	repo := NewProductRepository(db)

	err := repo.ReserveStock(
		context.Background(),
		1,
		3,
	)

	if err != nil {
		t.Fatalf(
			"failed to prepare stock: %v",
			err,
		)
	}

	body := bytes.NewBufferString(
		`{"quantity":3}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/products/1/release",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	releaseStock(rec, req, repo)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			rec.Code,
		)
	}

	var product Product

	if err := json.NewDecoder(rec.Body).Decode(&product); err != nil {
		t.Fatalf(
			"failed to decode response: %v",
			err,
		)
	}

	if product.Stock != 10 {
		t.Fatalf(
			"expected stock 10, got %d",
			product.Stock,
		)
	}

	resetInventoryHTTPTestProduct(t, db)
}

func TestReleaseStockInvalidQuantity(t *testing.T) {
	db := newInventoryHTTPTestDB(t)
	repo := NewProductRepository(db)

	body := bytes.NewBufferString(
		`{"quantity":0}`,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/products/1/release",
		body,
	)

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	rec := httptest.NewRecorder()

	releaseStock(rec, req, repo)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status 400, got %d",
			rec.Code,
		)
	}
}
