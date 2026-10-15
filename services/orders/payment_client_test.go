package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestPaymentClientAuthorizationAndRefundAreRetrySafe(t *testing.T) {
	var mu sync.Mutex
	payment := Payment{ID: 7, OrderID: 42, Status: PaymentStatusPending, AmountCents: 2500}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/payments/7":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payment)
		case r.Method == http.MethodGet && r.URL.Path == "/payments/order/42":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payment)
		case r.Method == http.MethodPost && r.URL.Path == "/payments/7/authorize":
			if payment.Status != PaymentStatusPending {
				http.Error(w, "invalid state", http.StatusConflict)
				return
			}
			payment.Status = PaymentStatusAuthorized
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payment)
		case r.Method == http.MethodPost && r.URL.Path == "/payments/7/refund":
			if payment.Status != PaymentStatusAuthorized {
				http.Error(w, "invalid state", http.StatusConflict)
				return
			}
			payment.Status = PaymentStatusRefunded
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payment)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewPaymentClient(server.URL, server.Client())
	ctx := context.Background()

	for attempt := 0; attempt < 2; attempt++ {
		result, err := client.AuthorizePayment(ctx, 7)
		if err != nil {
			t.Fatalf("authorization attempt %d: %v", attempt+1, err)
		}
		if result.Status != PaymentStatusAuthorized {
			t.Fatalf("expected authorized status, got %s", result.Status)
		}
	}

	result, err := client.GetPaymentForOrder(ctx, 42)
	if err != nil {
		t.Fatalf("get payment for order: %v", err)
	}
	if result.ID != 7 || result.OrderID != 42 {
		t.Fatalf("unexpected payment returned: %+v", result)
	}

	for attempt := 0; attempt < 2; attempt++ {
		result, err := client.RefundPayment(ctx, 7)
		if err != nil {
			t.Fatalf("refund attempt %d: %v", attempt+1, err)
		}
		if result.Status != PaymentStatusRefunded {
			t.Fatalf("expected refunded status, got %s", result.Status)
		}
	}
}
