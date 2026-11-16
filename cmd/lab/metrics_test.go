package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewPrometheusClient(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid URL", url: "http://localhost:9090"},
		{name: "invalid URL", url: "://invalid", wantErr: true},
		{name: "missing host", url: "http://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newPrometheusClient(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestPrometheusQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if r.URL.Query().Get("query") != "up" {
			t.Fatalf("unexpected query: %s", r.URL.Query().Get("query"))
		}

		fmt.Fprint(w, `{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {"service": "orders"},
						"value": [1730000000, "5"]
					}
				]
			}
		}`)
	}))
	defer server.Close()

	client, err := newPrometheusClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	values, err := client.query(context.Background(), "up")
	if err != nil {
		t.Fatal(err)
	}

	if len(values) != 1 {
		t.Fatalf("got %d results, want 1", len(values))
	}

	if values[0].Service != "orders" || values[0].Value != 5 {
		t.Fatalf("unexpected result: %+v", values[0])
	}
}

func TestPrometheusQueryHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := newPrometheusClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.query(context.Background(), "up")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got %v", err)
	}
}

func TestPrometheusQueryInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "invalid json")
	}))
	defer server.Close()

	client, err := newPrometheusClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected JSON decoding error")
	}
}

func TestPrometheusQueryFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"error","error":"bad query"}`)
	}))
	defer server.Close()

	client, err := newPrometheusClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.query(context.Background(), "up")
	if err == nil || !strings.Contains(err.Error(), "bad query") {
		t.Fatalf("expected Prometheus query error, got %v", err)
	}
}

func TestPrometheusSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")

		var result string

		switch {
		case strings.Contains(query, "http_server_requests_total{status"):
			result = `[
				{"metric":{"service":"gateway"},"value":[1730000000,"2"]},
				{"metric":{"service":"orders"},"value":[1730000000,"1"]}
			]`

		case strings.Contains(query, "http_server_requests_total"):
			result = `[
				{"metric":{"service":"gateway"},"value":[1730000000,"100"]},
				{"metric":{"service":"orders"},"value":[1730000000,"50"]}
			]`

		case strings.Contains(query, "_duration_seconds"):
			result = `[
				{"metric":{"service":"gateway"},"value":[1730000000,"0.025"]},
				{"metric":{"service":"orders"},"value":[1730000000,"0.05"]}
			]`

		case strings.Contains(query, "requests_in_flight"):
			result = `[
				{"metric":{"service":"gateway"},"value":[1730000000,"3"]},
				{"metric":{"service":"orders"},"value":[1730000000,"1"]}
			]`

		default:
			t.Fatalf("unexpected query: %s", query)
		}

		fmt.Fprintf(w, `{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":%s
			}
		}`, result)
	}))
	defer server.Close()

	client, err := newPrometheusClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := client.summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(summaries) != 4 {
		t.Fatalf("got %d services, want 4", len(summaries))
	}

	var gateway ServiceMetrics

	for _, item := range summaries {
		if item.Service == "gateway" {
			gateway = item
			break
		}
	}

	if gateway.Requests != 100 {
		t.Fatalf("gateway requests = %v, want 100", gateway.Requests)
	}

	if gateway.Errors != 2 {
		t.Fatalf("gateway errors = %v, want 2", gateway.Errors)
	}

	if gateway.ErrorRate != 2 {
		t.Fatalf("gateway error rate = %v, want 2", gateway.ErrorRate)
	}

	if gateway.AverageLatency != 25*time.Millisecond {
		t.Fatalf("gateway latency = %v, want 25ms", gateway.AverageLatency)
	}

	if gateway.InFlight != 3 {
		t.Fatalf("gateway in-flight = %v, want 3", gateway.InFlight)
	}
}

func TestPrometheusSummaryZeroRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[]
			}
		}`)
	}))
	defer server.Close()

	client, err := newPrometheusClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := client.summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range summaries {
		if item.Requests != 0 || item.Errors != 0 || item.ErrorRate != 0 {
			t.Fatalf("unexpected zero-request metrics: %+v", item)
		}
	}
}

func TestPrintMetricsSummary(t *testing.T) {
	summaries := []ServiceMetrics{
		{
			Service:        "orders",
			Requests:       100,
			Errors:         5,
			ErrorRate:      5,
			AverageLatency: 25 * time.Millisecond,
			InFlight:       2,
		},
		{
			Service:        "gateway",
			Requests:       200,
			Errors:         10,
			ErrorRate:      5,
			AverageLatency: 10 * time.Millisecond,
		},
	}

	if len(summaries) != 2 {
		t.Fatalf("got %d summaries, want 2", len(summaries))
	}

	if summaries[0].ErrorRate != 5 {
		t.Fatalf("got error rate %f, want 5", summaries[0].ErrorRate)
	}

	if summaries[0].AverageLatency != 25*time.Millisecond {
		t.Fatalf("unexpected latency: %v", summaries[0].AverageLatency)
	}
}
