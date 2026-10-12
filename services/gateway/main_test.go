package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayHealth(t *testing.T) {
	handler, err := newGatewayHandler("http://localhost:8081")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "{\"status\":\"ok\"}" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestGatewayLiveness(t *testing.T) {
	handler, err := newGatewayHandler("http://localhost:8081")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/liveness", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "{\"status\":\"alive\"}" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestGatewayHealthRejectsPost(t *testing.T) {
	handler, err := newGatewayHandler("http://localhost:8081")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestGatewayForwardsOrderRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		if r.URL.Path != "/orders/123" {
			t.Errorf("expected path /orders/123, got %s", r.URL.Path)
		}

		if r.URL.Query().Get("include") != "items" {
			t.Errorf("expected query include=items, got %s", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":123}`)
	}))
	defer upstream.Close()

	handler, err := newGatewayHandler(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/orders/123?include=items", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "{\"id\":123}" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestGatewayReturnsBadGatewayWhenOrdersUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	targetURL := upstream.URL
	upstream.Close()

	handler, err := newGatewayHandler(targetURL)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/orders/123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rec.Code)
	}
}

func TestGatewayRejectsInvalidOrdersURL(t *testing.T) {
	_, err := newGatewayHandler("://invalid")
	if err == nil {
		t.Fatal("expected invalid Orders URL to return an error")
	}
}
