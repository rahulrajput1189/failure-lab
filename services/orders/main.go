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

type CreateOrderRequest struct {
	UserID    int64 `json:"user_id"`
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

func writeOrderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidUser),
		errors.Is(err, ErrInvalidProduct),
		errors.Is(err, ErrInsufficientStock):
		http.Error(w, err.Error(), http.StatusBadRequest)

	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "order not found", http.StatusNotFound)

	default:
		log.Printf("order request failed: %v", err)

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
		writeOrderError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(order)
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

	order, err := orderService.GetOrder(
		r.Context(),
		orderID,
	)

	if err != nil {
		writeOrderError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(order); err != nil {
		log.Printf("failed to encode order response: %v", err)
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

	order, err := orderService.CancelOrder(
		r.Context(),
		orderID,
	)

	if err != nil {
		writeOrderError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(order); err != nil {
		log.Printf("failed to encode cancelled order response: %v", err)
	}
}

func health(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool) {
	w.Header().Set("Content-Type", "application/json")

	if err := db.Ping(r.Context()); err != nil {
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

	orderService.failAfterInventoryReserve = config.FailAfterInventoryReserve

	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(
				w,
				"method not allowed",
				http.StatusMethodNotAllowed,
			)
			return
		}

		createOrder(w, r, orderService)
	})

	http.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getOrder(w, r, orderService)

		case http.MethodDelete:
			cancelOrder(w, r, orderService)

		default:
			http.Error(
				w,
				"method not allowed",
				http.StatusMethodNotAllowed,
			)
		}
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health(w, r, db)
	})

	http.HandleFunc("/liveness", liveness)

	log.Printf("orders service listening on :%s", config.HTTPPort)

	if err := http.ListenAndServe(":"+config.HTTPPort, nil); err != nil {
		log.Fatal(err)
	}
}
