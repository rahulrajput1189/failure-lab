package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StockRequest struct {
	Quantity int `json:"quantity"`
}

func writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(
			w,
			"product not found",
			http.StatusNotFound,
		)
		return
	}

	if strings.Contains(err.Error(), "insufficient stock") {
		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	log.Printf("inventory request failed: %v", err)

	http.Error(
		w,
		"internal server error",
		http.StatusInternalServerError,
	)
}

func parseProductID(path string) (int64, error) {
	path = strings.TrimPrefix(path, "/products/")
	path = strings.TrimSuffix(path, "/reserve")
	path = strings.TrimSuffix(path, "/release")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		return 0, fmt.Errorf("missing product ID")
	}

	productID, err := strconv.ParseInt(path, 10, 64)
	if err != nil || productID <= 0 {
		return 0, fmt.Errorf("invalid product ID")
	}

	return productID, nil
}

func getProduct(
	w http.ResponseWriter,
	r *http.Request,
	productRepo *ProductRepository,
) {
	productID, err := parseProductID(r.URL.Path)
	if err != nil {
		http.Error(
			w,
			"invalid product ID",
			http.StatusBadRequest,
		)
		return
	}

	product, err := productRepo.GetByID(
		r.Context(),
		productID,
	)

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(product); err != nil {
		log.Printf("failed to encode product response: %v", err)
	}
}

func reserveStock(
	w http.ResponseWriter,
	r *http.Request,
	productRepo *ProductRepository,
) {
	var req StockRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(
			w,
			"invalid JSON",
			http.StatusBadRequest,
		)
		return
	}

	if req.Quantity <= 0 {
		http.Error(
			w,
			"quantity must be greater than zero",
			http.StatusBadRequest,
		)
		return
	}

	productID, err := parseProductID(r.URL.Path)
	if err != nil {
		http.Error(
			w,
			"invalid product ID",
			http.StatusBadRequest,
		)
		return
	}

	if err := productRepo.ReserveStock(
		r.Context(),
		productID,
		req.Quantity,
	); err != nil {
		writeError(w, err)
		return
	}

	product, err := productRepo.GetByID(
		r.Context(),
		productID,
	)

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(product); err != nil {
		log.Printf("failed to encode product response: %v", err)
	}
}

func releaseStock(
	w http.ResponseWriter,
	r *http.Request,
	productRepo *ProductRepository,
) {
	var req StockRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(
			w,
			"invalid JSON",
			http.StatusBadRequest,
		)
		return
	}

	if req.Quantity <= 0 {
		http.Error(
			w,
			"quantity must be greater than zero",
			http.StatusBadRequest,
		)
		return
	}

	productID, err := parseProductID(r.URL.Path)
	if err != nil {
		http.Error(
			w,
			"invalid product ID",
			http.StatusBadRequest,
		)
		return
	}

	if err := productRepo.ReleaseStock(
		r.Context(),
		productID,
		req.Quantity,
	); err != nil {
		writeError(w, err)
		return
	}

	product, err := productRepo.GetByID(
		r.Context(),
		productID,
	)

	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(product); err != nil {
		log.Printf("failed to encode product response: %v", err)
	}
}

func health(
	w http.ResponseWriter,
	r *http.Request,
	db *pgxpool.Pool,
) {
	w.Header().Set("Content-Type", "application/json")

	if err := db.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"not_ready"}`)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

func liveness(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprint(w, `{"status":"alive"}`)
}

func main() {
	config := loadConfig()

	db := newDatabasePool(config)
	defer db.Close()

	productRepo := NewProductRepository(db)

	http.HandleFunc("/products/", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		path := strings.TrimSuffix(r.URL.Path, "/")

		switch {
		case r.Method == http.MethodGet &&
			strings.Count(path, "/") == 2:
			getProduct(w, r, productRepo)

		case r.Method == http.MethodPost &&
			strings.HasSuffix(path, "/reserve"):
			reserveStock(w, r, productRepo)

		case r.Method == http.MethodPost &&
			strings.HasSuffix(path, "/release"):
			releaseStock(w, r, productRepo)

		default:
			http.Error(
				w,
				"not found",
				http.StatusNotFound,
			)
		}
	})

	http.HandleFunc("/health", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		health(w, r, db)
	})

	http.HandleFunc("/liveness", liveness)

	log.Printf(
		"inventory service listening on :%s",
		config.HTTPPort,
	)

	if err := http.ListenAndServe(
		":"+config.HTTPPort,
		nil,
	); err != nil {
		log.Fatal(err)
	}
}
