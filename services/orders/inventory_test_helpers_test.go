package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type testInventoryServer struct {
	server *httptest.Server
	mu     sync.Mutex
	stock  map[int64]int
}

func newTestInventoryServer(t *testing.T) *testInventoryServer {
	t.Helper()

	inventory := &testInventoryServer{
		stock: map[int64]int{
			1: 10,
			2: 50,
			3: 100,
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/products/", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		path := strings.TrimPrefix(
			r.URL.Path,
			"/products/",
		)

		parts := strings.Split(
			strings.TrimSuffix(path, "/"),
			"/",
		)

		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}

		productID, err := strconv.ParseInt(
			parts[0],
			10,
			64,
		)

		if err != nil {
			http.Error(
				w,
				"invalid product ID",
				http.StatusBadRequest,
			)
			return
		}

		if len(parts) == 1 &&
			r.Method == http.MethodGet {
			inventory.getProduct(
				w,
				productID,
			)
			return
		}

		if len(parts) == 2 &&
			parts[1] == "reserve" &&
			r.Method == http.MethodPost {
			inventory.reserveStock(
				w,
				r,
				productID,
			)
			return
		}

		http.NotFound(w, r)
	})

	inventory.server = httptest.NewServer(mux)

	t.Cleanup(func() {
		inventory.server.Close()
	})

	return inventory
}

func (s *testInventoryServer) getProduct(
	w http.ResponseWriter,
	productID int64,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stock, exists := s.stock[productID]

	if !exists {
		http.Error(
			w,
			"product not found",
			http.StatusNotFound,
		)
		return
	}

	product := InventoryProduct{
		ID:         productID,
		Name:       testProductName(productID),
		PriceCents: testProductPrice(productID),
		Stock:      stock,
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(product)
}

func (s *testInventoryServer) reserveStock(
	w http.ResponseWriter,
	r *http.Request,
	productID int64,
) {
	var request ReserveStockRequest

	if err := json.NewDecoder(
		r.Body,
	).Decode(&request); err != nil {
		http.Error(
			w,
			"invalid JSON",
			http.StatusBadRequest,
		)
		return
	}

	if request.Quantity <= 0 {
		http.Error(
			w,
			"quantity must be greater than zero",
			http.StatusBadRequest,
		)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stock, exists := s.stock[productID]

	if !exists {
		http.Error(
			w,
			"product not found",
			http.StatusNotFound,
		)
		return
	}

	if request.Quantity > stock {
		http.Error(
			w,
			"insufficient stock",
			http.StatusBadRequest,
		)
		return
	}

	s.stock[productID] -= request.Quantity

	product := InventoryProduct{
		ID:         productID,
		Name:       testProductName(productID),
		PriceCents: testProductPrice(productID),
		Stock:      s.stock[productID],
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(product)
}

func (s *testInventoryServer) client() *InventoryClient {
	return NewInventoryClient(
		s.server.URL,
		s.server.Client(),
	)
}

func (s *testInventoryServer) resetStock(
	productID int64,
	stock int,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stock[productID] = stock
}

func testProductName(productID int64) string {
	switch productID {
	case 1:
		return "Laptop"
	case 2:
		return "Keyboard"
	case 3:
		return "Mouse"
	default:
		return "Unknown"
	}
}

func testProductPrice(productID int64) int {
	switch productID {
	case 1:
		return 100000
	case 2:
		return 5000
	case 3:
		return 2500
	default:
		return 0
	}
}
