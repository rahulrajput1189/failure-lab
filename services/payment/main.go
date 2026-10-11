package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CreatePaymentRequest struct {
	OrderID     int64 `json:"order_id"`
	AmountCents int   `json:"amount_cents"`
}

func writePaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPaymentNotFound):
		http.Error(w, "payment not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidPaymentState):
		http.Error(w, err.Error(), http.StatusConflict)
	case strings.Contains(err.Error(), "must be greater than zero"):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		log.Printf("payment request failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writePaymentJSON(w http.ResponseWriter, status int, payment *Payment) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payment); err != nil {
		log.Printf("failed to encode payment response: %v", err)
	}
}

func createPayment(
	w http.ResponseWriter,
	r *http.Request,
	repo *PaymentRepository,
) {
	var req CreatePaymentRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	payment, err := repo.Create(r.Context(), req.OrderID, req.AmountCents)
	if err != nil {
		writePaymentError(w, err)
		return
	}

	writePaymentJSON(w, http.StatusCreated, payment)
}

func getPayment(
	w http.ResponseWriter,
	r *http.Request,
	repo *PaymentRepository,
	paymentID int64,
) {
	payment, err := repo.GetByID(r.Context(), paymentID)
	if err != nil {
		writePaymentError(w, err)
		return
	}

	writePaymentJSON(w, http.StatusOK, payment)
}

func transitionPayment(
	w http.ResponseWriter,
	r *http.Request,
	repo *PaymentRepository,
	paymentID int64,
	transition func(context.Context, *PaymentRepository, int64) (*Payment, error),
) {
	payment, err := transition(r.Context(), repo, paymentID)
	if err != nil {
		writePaymentError(w, err)
		return
	}

	writePaymentJSON(w, http.StatusOK, payment)
}

func paymentByID(
	w http.ResponseWriter,
	r *http.Request,
	repo *PaymentRepository,
) {
	path := strings.TrimPrefix(r.URL.Path, "/payments/")
	parts := strings.Split(path, "/")

	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	paymentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || paymentID <= 0 {
		http.Error(w, "invalid payment ID", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		getPayment(w, r, repo, paymentID)
		return
	}

	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	var handler func(context.Context, *PaymentRepository, int64) (*Payment, error)

	switch parts[1] {
	case "authorize":
		handler = func(ctx context.Context, repo *PaymentRepository, id int64) (*Payment, error) {
			return repo.Authorize(ctx, id)
		}
	case "refund":
		handler = func(ctx context.Context, repo *PaymentRepository, id int64) (*Payment, error) {
			return repo.Refund(ctx, id)
		}
	default:
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transitionPayment(w, r, repo, paymentID, handler)
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

func liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"alive"}`)
}

func main() {
	config := loadConfig()

	db := newDatabasePool(config)
	defer db.Close()

	paymentRepo := NewPaymentRepository(db)

	mux := http.NewServeMux()

	mux.HandleFunc("/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payments" {
			http.NotFound(w, r)
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		createPayment(w, r, paymentRepo)
	})

	mux.HandleFunc("/payments/", func(w http.ResponseWriter, r *http.Request) {
		paymentByID(w, r, paymentRepo)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		health(w, r, db)
	})

	mux.HandleFunc("/liveness", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		liveness(w, r)
	})

	server := &http.Server{
		Addr:              ":" + config.HTTPPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("payment service listening on :%s", config.HTTPPort)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
