package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInventoryClientGetProduct(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET, got %s", r.Method)
			}

			if r.URL.Path != "/products/1" {
				t.Fatalf(
					"expected /products/1, got %s",
					r.URL.Path,
				)
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			json.NewEncoder(w).Encode(
				InventoryProduct{
					ID:         1,
					Name:       "Laptop",
					PriceCents: 100000,
					Stock:      10,
				},
			)
		}),
	)

	defer server.Close()

	client := NewInventoryClient(
		server.URL,
		server.Client(),
	)

	product, err := client.GetProduct(
		context.Background(),
		1,
	)

	if err != nil {
		t.Fatalf(
			"GetProduct failed: %v",
			err,
		)
	}

	if product.ID != 1 {
		t.Fatalf(
			"expected product ID 1, got %d",
			product.ID,
		)
	}

	if product.Name != "Laptop" {
		t.Fatalf(
			"expected Laptop, got %s",
			product.Name,
		)
	}

	if product.PriceCents != 100000 {
		t.Fatalf(
			"expected price 100000, got %d",
			product.PriceCents,
		)
	}

	if product.Stock != 10 {
		t.Fatalf(
			"expected stock 10, got %d",
			product.Stock,
		)
	}
}

func TestInventoryClientReserveStock(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}

			if r.URL.Path != "/products/1/reserve" {
				t.Fatalf(
					"expected /products/1/reserve, got %s",
					r.URL.Path,
				)
			}

			var req ReserveStockRequest

			if err := json.NewDecoder(
				r.Body,
			).Decode(&req); err != nil {
				t.Fatalf(
					"failed to decode request: %v",
					err,
				)
			}

			if req.Quantity != 3 {
				t.Fatalf(
					"expected quantity 3, got %d",
					req.Quantity,
				)
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			json.NewEncoder(w).Encode(
				InventoryProduct{
					ID:         1,
					Name:       "Laptop",
					PriceCents: 100000,
					Stock:      7,
				},
			)
		}),
	)

	defer server.Close()

	client := NewInventoryClient(
		server.URL,
		server.Client(),
	)

	product, err := client.ReserveStock(
		context.Background(),
		1,
		3,
	)

	if err != nil {
		t.Fatalf(
			"ReserveStock failed: %v",
			err,
		)
	}

	if product.Stock != 7 {
		t.Fatalf(
			"expected stock 7, got %d",
			product.Stock,
		)
	}
}

func TestInventoryClientRejectsNonOKResponse(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(
				w,
				"insufficient stock",
				http.StatusBadRequest,
			)
		}),
	)

	defer server.Close()

	client := NewInventoryClient(
		server.URL,
		server.Client(),
	)

	_, err := client.ReserveStock(
		context.Background(),
		1,
		100,
	)

	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}
