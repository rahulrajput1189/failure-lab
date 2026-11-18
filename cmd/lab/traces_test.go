package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewJaegerClient(t *testing.T) {
	client, err := newJaegerClient("http://localhost:16686/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.baseURL != "http://localhost:16686" {
		t.Fatalf("unexpected base URL: %s", client.baseURL)
	}

	if _, err := newJaegerClient("invalid-url"); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestJaegerServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(jaegerServicesResponse{
			Data: []string{"orders", "jaeger-all-in-one", "inventory"},
		})
	}))
	defer server.Close()

	client, _ := newJaegerClient(server.URL)

	services, err := client.services(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"inventory", "orders"}
	if len(services) != len(expected) {
		t.Fatalf("expected %d services, got %d", len(expected), len(services))
	}

	for i := range expected {
		if services[i] != expected[i] {
			t.Fatalf("expected %q, got %q", expected[i], services[i])
		}
	}
}

func TestJaegerTraces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if r.URL.Query().Get("service") != "orders" {
			t.Fatalf("unexpected service: %s", r.URL.Query().Get("service"))
		}

		if r.URL.Query().Get("lookback") != "1h" {
			t.Fatalf("unexpected lookback: %s", r.URL.Query().Get("lookback"))
		}

		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("unexpected limit: %s", r.URL.Query().Get("limit"))
		}

		json.NewEncoder(w).Encode(jaegerTracesResponse{
			Data: []jaegerTrace{{TraceID: "trace-1"}},
		})
	}))
	defer server.Close()

	client, _ := newJaegerClient(server.URL)

	traces, err := client.traces(context.Background(), "orders", "1h", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(traces) != 1 || traces[0].TraceID != "trace-1" {
		t.Fatalf("unexpected traces: %+v", traces)
	}
}

func TestJaegerHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Jaeger unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, _ := newJaegerClient(server.URL)

	_, err := client.services(context.Background())
	if err == nil {
		t.Fatal("expected HTTP error")
	}

	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJaegerInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{invalid"))
	}))
	defer server.Close()

	client, _ := newJaegerClient(server.URL)

	_, err := client.services(context.Background())
	if err == nil {
		t.Fatal("expected JSON decoding error")
	}
}

func TestSummarizeTrace(t *testing.T) {
	trace := jaegerTrace{
		TraceID: "trace-123",
		Spans: []jaegerSpan{
			{
				TraceID:       "trace-123",
				SpanID:        "root",
				OperationName: "POST",
				StartTime:     200,
				Duration:      35805,
				ProcessID:     "p1",
				Tags: []jaegerTag{
					{Key: "http.request.method", Value: "POST"},
					{Key: "url.path", Value: "/orders"},
				},
			},
			{
				TraceID:       "trace-123",
				SpanID:        "child",
				OperationName: "GET /products",
				StartTime:     210,
				Duration:      10000,
				ProcessID:     "p2",
				References:    []any{map[string]any{"refType": "CHILD_OF"}},
			},
		},
		Processes: map[string]jaegerProcess{
			"p1": {ServiceName: "orders"},
			"p2": {ServiceName: "inventory"},
		},
	}

	summary, ok := summarizeTrace(trace)
	if !ok {
		t.Fatal("expected trace summary")
	}

	if summary.TraceID != "trace-123" {
		t.Fatalf("unexpected trace ID: %s", summary.TraceID)
	}
	if summary.Service != "orders" {
		t.Fatalf("unexpected root service: %s", summary.Service)
	}
	if summary.Operation != "POST /orders" {
		t.Fatalf("unexpected operation: %s", summary.Operation)
	}
	if summary.Duration != 35805*time.Microsecond {
		t.Fatalf("unexpected duration: %s", summary.Duration)
	}
	if summary.SpanCount != 2 {
		t.Fatalf("expected 2 spans, got %d", summary.SpanCount)
	}
	if len(summary.Services) != 2 ||
		summary.Services[0] != "inventory" ||
		summary.Services[1] != "orders" {
		t.Fatalf("unexpected participating services: %v", summary.Services)
	}
}

func TestSummarizeTraceWithoutRoot(t *testing.T) {
	trace := jaegerTrace{
		TraceID: "trace-no-root",
		Spans: []jaegerSpan{
			{
				SpanID:     "child",
				References: []any{map[string]any{"refType": "CHILD_OF"}},
			},
		},
	}

	if _, ok := summarizeTrace(trace); ok {
		t.Fatal("expected trace without root to be skipped")
	}
}

func TestSummarizeTraceWithoutSpans(t *testing.T) {
	if _, ok := summarizeTrace(jaegerTrace{TraceID: "empty"}); ok {
		t.Fatal("expected empty trace to be skipped")
	}
}

func TestJaegerSummaryDeduplicatesAndSorts(t *testing.T) {
	traceOne := jaegerTrace{
		TraceID: "trace-one",
		Spans: []jaegerSpan{
			{
				SpanID:        "root-one",
				OperationName: "GET /health",
				StartTime:     100,
				Duration:      1000,
				ProcessID:     "p1",
			},
		},
		Processes: map[string]jaegerProcess{
			"p1": {ServiceName: "orders"},
		},
	}

	traceTwo := jaegerTrace{
		TraceID: "trace-two",
		Spans: []jaegerSpan{
			{
				SpanID:        "root-two",
				OperationName: "POST",
				StartTime:     300,
				Duration:      2000,
				ProcessID:     "p1",
				Tags: []jaegerTag{
					{Key: "http.request.method", Value: "POST"},
					{Key: "url.path", Value: "/orders"},
				},
			},
		},
		Processes: map[string]jaegerProcess{
			"p1": {ServiceName: "orders"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/services":
			json.NewEncoder(w).Encode(jaegerServicesResponse{
				Data: []string{"orders", "inventory"},
			})
		case "/api/traces":
			if r.URL.Query().Get("service") == "orders" {
				json.NewEncoder(w).Encode(jaegerTracesResponse{
					Data: []jaegerTrace{traceOne, traceTwo},
				})
				return
			}

			// Jaeger can return the same distributed trace for multiple services.
			json.NewEncoder(w).Encode(jaegerTracesResponse{
				Data: []jaegerTrace{traceTwo},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, _ := newJaegerClient(server.URL)

	summaries, err := client.summary(context.Background(), "1h", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 unique traces, got %d", len(summaries))
	}

	if summaries[0].TraceID != "trace-two" {
		t.Fatalf("expected newest trace first, got %s", summaries[0].TraceID)
	}
	if summaries[1].TraceID != "trace-one" {
		t.Fatalf("expected older trace second, got %s", summaries[1].TraceID)
	}
}

func TestRootOperationFallback(t *testing.T) {
	span := jaegerSpan{
		OperationName: "GET /health",
		Tags: []jaegerTag{
			{Key: "http.request.method", Value: "GET"},
		},
	}

	if got := rootOperation(span); got != "GET /health" {
		t.Fatalf("unexpected fallback operation: %s", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("POST /orders", 8); got != "POST ..." {
		t.Fatalf("unexpected truncated value: %q", got)
	}

	if got := truncate("GET", 8); got != "GET" {
		t.Fatalf("unexpected unchanged value: %q", got)
	}
}
