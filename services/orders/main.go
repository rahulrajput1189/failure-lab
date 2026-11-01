package main

import (
	"context"
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

type CreateOrderRequest struct {
	UserID    int64 `json:"user_id"`
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}
type requestIDContextKey struct{}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}

		r.Header.Set(requestIDHeader, requestID)
		w.Header().Set(requestIDHeader, requestID)

		r = r.WithContext(contextWithRequestID(r.Context(), requestID))

		slog.Info("request received",
			"service", "orders",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		)
		next.ServeHTTP(w, r)
	})
}

func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func requestIDFromRequest(r *http.Request) string {
	return r.Header.Get(requestIDHeader)
}

func writeOrderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidUser),
		errors.Is(err, ErrInvalidProduct),
		errors.Is(err, ErrInsufficientStock):
		http.Error(w, err.Error(), http.StatusBadRequest)

	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "order not found", http.StatusNotFound)

	case errors.Is(err, ErrInvalidOrderState):
		http.Error(w, err.Error(), http.StatusConflict)

	default:
		slog.Error("order request failed",
			"service", "orders",
			"request_id", requestIDFromRequest(r),
			"error", err,
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
	}
}

func createOrder(
	w http.ResponseWriter,
	r *http.Request,
	orderService *OrderService,
) {
	var req CreateOrderRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	order, err := orderService.CreateOrder(
		r.Context(),
		CreateOrderInput{
			UserID:    req.UserID,
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
		},
	)

	if err != nil {
		writeOrderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(order); err != nil {
		slog.Error("failed to encode order response",
			"service", "orders",
			"request_id", requestIDFromRequest(r),
			"error", err,
		)
	}
}

func getOrder(
	w http.ResponseWriter,
	r *http.Request,
	orderService *OrderService,
) {
	orderIDText := strings.TrimPrefix(r.URL.Path, "/orders/")

	if orderIDText == "" {
		http.Error(w, "invalid order ID", http.StatusBadRequest)
		return
	}

	orderID, err := strconv.ParseInt(orderIDText, 10, 64)
	if err != nil || orderID <= 0 {
		http.Error(w, "invalid order ID", http.StatusBadRequest)
		return
	}

	order, err := orderService.GetOrder(r.Context(), orderID)
	if err != nil {
		writeOrderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(order); err != nil {
		slog.Error("failed to encode order response",
			"service", "orders",
			"request_id", requestIDFromRequest(r),
			"order_id", orderID,
			"error", err,
		)
	}
}

func cancelOrder(
	w http.ResponseWriter,
	r *http.Request,
	orderService *OrderService,
) {
	orderIDText := strings.TrimPrefix(r.URL.Path, "/orders/")

	if orderIDText == "" {
		http.Error(w, "invalid order ID", http.StatusBadRequest)
		return
	}

	orderID, err := strconv.ParseInt(orderIDText, 10, 64)
	if err != nil || orderID <= 0 {
		http.Error(w, "invalid order ID", http.StatusBadRequest)
		return
	}

	order, err := orderService.CancelOrder(r.Context(), orderID)
	if err != nil {
		writeOrderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(order); err != nil {
		slog.Error("failed to encode cancelled order response",
			"service", "orders",
			"request_id", requestIDFromRequest(r),
			"order_id", orderID,
			"error", err,
		)
	}
}

func health(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool) {
	w.Header().Set("Content-Type", "application/json")

	if err := db.Ping(r.Context()); err != nil {
		slog.Error("health check failed",
			"service", "orders",
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

func liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"alive"}`)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	config := loadConfig()
	db := newDatabasePool(config)
	defer db.Close()

	userRepo := NewUserRepository(db)
	orderRepo := NewOrderRepository(db)
	orderItemRepo := NewOrderItemRepository(db)

	inventoryClient := NewInventoryClient(
		"http://localhost:8082",
		http.DefaultClient,
	)

	orderService := NewOrderService(
		db,
		userRepo,
		orderRepo,
		orderItemRepo,
		inventoryClient,
	)

	paymentClient := NewPaymentClient(
		config.PaymentServiceURL,
		http.DefaultClient,
	)
	orderService.SetPaymentClient(paymentClient)

	orderService.failAfterInventoryReserve = config.FailAfterInventoryReserve
	orderService.failAfterInventoryRelease = config.FailAfterInventoryRelease

	mux := http.NewServeMux()

	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		createOrder(w, r, orderService)
	})

	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getOrder(w, r, orderService)
		case http.MethodDelete:
			cancelOrder(w, r, orderService)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health(w, r, db)
	})

	mux.HandleFunc("/liveness", liveness)

	serviceMetrics := metrics.New("orders")
	mux.Handle("/metrics", serviceMetrics.Handler())

	handler := requestIDMiddleware(mux)

	handler = serviceMetrics.Middleware(metrics.RoutePattern, handler)

	slog.Info("orders service listening",
		"service", "orders",
		"port", config.HTTPPort,
	)

	if err := http.ListenAndServe(":"+config.HTTPPort, handler); err != nil {
		slog.Error("orders server failed",
			"service", "orders",
			"error", err,
		)
		os.Exit(1)
	}
}
