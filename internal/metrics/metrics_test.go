package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMiddlewareRecordsRequestMetrics(t *testing.T) {
	m := New("test-service")

	handler := m.Middleware(
		func(r *http.Request) string {
			return "/orders"
		},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/orders", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if got := testutil.ToFloat64(
		m.requestsTotal.WithLabelValues("POST", "/orders", "201"),
	); got != 1 {
		t.Fatalf("request count = %v, want 1", got)
	}

	if got := testutil.ToFloat64(m.requestsInFlight); got != 0 {
		t.Fatalf("in-flight requests = %v, want 0", got)
	}

	// var output strings.Builder
	// if _, err := m.registry.WriteTo(&output); err == nil {
	// 	t.Fatal("unexpected registry.WriteTo method")
	// }
}

func TestMiddlewareUsesUnmatchedRouteWhenPatternIsEmpty(t *testing.T) {
	m := New("test-service")

	handler := m.Middleware(
		func(r *http.Request) string { return "" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/unknown", nil),
	)

	if got := testutil.ToFloat64(
		m.requestsTotal.WithLabelValues("GET", "unmatched", "404"),
	); got != 1 {
		t.Fatalf("request count = %v, want 1", got)
	}
}

func TestMiddlewareSupportsConcurrentRequests(t *testing.T) {
	m := New("test-service")

	handler := m.Middleware(
		func(r *http.Request) string { return "/health" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	const requests = 100
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodGet, "/health", nil),
			)
		}()
	}

	wg.Wait()

	if got := testutil.ToFloat64(
		m.requestsTotal.WithLabelValues("GET", "/health", "200"),
	); got != requests {
		t.Fatalf("request count = %v, want %d", got, requests)
	}
}

func TestMetricsHandlerExposesPrometheusFormat(t *testing.T) {
	m := New("test-service")

	handler := m.Middleware(
		func(r *http.Request) string { return "/health" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/health", nil),
	)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, expected := range []string{
		"http_server_requests_total",
		"http_server_request_duration_seconds",
		"http_server_requests_in_flight",
		`service="test-service"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("metrics output missing %q", expected)
		}
	}
}
