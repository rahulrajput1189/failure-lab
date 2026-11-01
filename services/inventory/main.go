package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"failure-lab/internal/metrics"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const requestIDHeader = "X-Request-ID"

type StockRequest struct {
	Quantity int `json:"quantity"`
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}

		r.Header.Set(requestIDHeader, requestID)
		w.Header().Set(requestIDHeader, requestID)

		slog.Info("request received",
			"service", "inventory",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		)

		next.ServeHTTP(w, r)
	})
}

func requestIDFromRequest(r *http.Request) string {
	return r.Header.Get(requestIDHeader)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrInventoryIdempotencyConflict) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "insufficient stock") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slog.Error("inventory request failed",
		"service", "inventory",
		"request_id", requestIDFromRequest(r),
		"error", err,
	)

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
		http.Error(w, "invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := productRepo.GetByID(r.Context(), productID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(product); err != nil {
		slog.Error("failed to encode product response",
			"service", "inventory",
			"request_id", requestIDFromRequest(r),
			"error", err,
			"product_id", productID,
		)
	}
}

func reserveStock(
	w http.ResponseWriter,
	r *http.Request,
	productRepo *ProductRepository,
) {
	var req StockRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Quantity <= 0 {
		http.Error(w, "quantity must be greater than zero", http.StatusBadRequest)
		return
	}

	productID, err := parseProductID(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid product ID", http.StatusBadRequest)
		return
	}

	if err := productRepo.ReserveStock(r.Context(), productID, req.Quantity); err != nil {
		writeError(w, r, err)
		return
	}

	product, err := productRepo.GetByID(r.Context(), productID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(product); err != nil {
		slog.Error("failed to encode product response",
			"service", "inventory",
			"request_id", requestIDFromRequest(r),
			"error", err,
			"product_id", productID,
		)
	}
}

func releaseStock(
	w http.ResponseWriter,
	r *http.Request,
	productRepo *ProductRepository,
) {
	var req StockRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Quantity <= 0 {
		http.Error(w, "quantity must be greater than zero", http.StatusBadRequest)
		return
	}

	productID, err := parseProductID(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid product ID", http.StatusBadRequest)
		return
	}

	if operationKey := r.Header.Get("Idempotency-Key"); operationKey != "" {
		err = productRepo.ReleaseStockIdempotent(
			r.Context(),
			operationKey,
			productID,
			req.Quantity,
		)
	} else {
		err = productRepo.ReleaseStock(
			r.Context(),
			productID,
			req.Quantity,
		)
	}

	if err != nil {
		writeError(w, r, err)
		return
	}

	product, err := productRepo.GetByID(r.Context(), productID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(product); err != nil {
		slog.Error("failed to encode product response",
			"service", "inventory",
			"request_id", requestIDFromRequest(r),
			"error", err,
			"product_id", productID,
		)
	}
}

func health(
	w http.ResponseWriter,
	r *http.Request,
	db *pgxpool.Pool,
) {
	w.Header().Set("Content-Type", "application/json")

	if err := db.Ping(r.Context()); err != nil {
		slog.Error("health check failed",
			"service", "inventory",
			"request_id", requestIDFromRequest(r),
			"error", err,
		)
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	config := loadConfig()

	db := newDatabasePool(config)
	defer db.Close()

	productRepo := NewProductRepository(db)

	mux := http.NewServeMux()

	mux.HandleFunc("/products/", func(
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
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	mux.HandleFunc("/health", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		health(w, r, db)
	})

	mux.HandleFunc("/liveness", liveness)

	serviceMetrics := metrics.New("inventory")
	mux.Handle("/metrics", serviceMetrics.Handler())

	handler := requestIDMiddleware(mux)

	handler = serviceMetrics.Middleware(metrics.RoutePattern, handler)

	slog.Info("service listening",
		"service", "inventory",
		"port", config.HTTPPort,
	)

	if err := http.ListenAndServe(":"+config.HTTPPort, handler); err != nil {
		slog.Error("HTTP server failed",
			"service", "inventory",
			"error", err,
		)
		os.Exit(1)
	}
}
