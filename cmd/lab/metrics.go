package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultPrometheusURL = "http://localhost:9090"

var metricServices = []string{
	"gateway",
	"orders",
	"inventory",
	"payment",
}

type ServiceMetrics struct {
	Service        string
	Requests       float64
	Errors         float64
	ErrorRate      float64
	AverageLatency time.Duration
	InFlight       float64
}

type prometheusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type prometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

func newPrometheusClient(baseURL string) (*prometheusClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Prometheus URL: %q", baseURL)
	}

	return &prometheusClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (c *prometheusClient) query(ctx context.Context, expression string) ([]struct {
	Service string
	Value   float64
}, error) {
	endpoint := c.baseURL + "/api/v1/query"

	requestURL := endpoint + "?query=" + url.QueryEscape(expression)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not connect to Prometheus: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf(
			"Prometheus returned HTTP %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var result prometheusResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("could not decode Prometheus response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("Prometheus query failed: %s", result.Error)
	}

	values := make([]struct {
		Service string
		Value   float64
	}, 0, len(result.Data.Result))

	for _, item := range result.Data.Result {
		if len(item.Value) != 2 {
			continue
		}

		var rawValue string
		if err := json.Unmarshal(item.Value[1], &rawValue); err != nil {
			continue
		}

		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil {
			continue
		}

		values = append(values, struct {
			Service string
			Value   float64
		}{
			Service: item.Metric["service"],
			Value:   value,
		})
	}

	return values, nil
}

func (c *prometheusClient) summary(ctx context.Context) ([]ServiceMetrics, error) {
	queries := []struct {
		name string
		expr string
	}{
		{
			name: "requests",
			expr: `sum by (service) (http_server_requests_total)`,
		},
		{
			name: "errors",
			expr: `sum by (service) (http_server_requests_total{status=~"5.."})`,
		},
		{
			name: "latency",
			expr: `sum by (service) (http_server_request_duration_seconds_sum) / sum by (service) (http_server_request_duration_seconds_count)`,
		},
		{
			name: "in_flight",
			expr: `sum by (service) (http_server_requests_in_flight)`,
		},
	}

	results := make(map[string]map[string]float64)

	for _, query := range queries {
		values, err := c.query(ctx, query.expr)
		if err != nil {
			return nil, fmt.Errorf("querying %s metrics: %w", query.name, err)
		}

		for _, item := range values {
			if results[item.Service] == nil {
				results[item.Service] = make(map[string]float64)
			}
			results[item.Service][query.name] = item.Value
		}
	}

	summaries := make([]ServiceMetrics, 0, len(metricServices))

	for _, service := range metricServices {
		values := results[service]

		requests := values["requests"]
		errors := values["errors"]

		errorRate := float64(0)
		if requests > 0 {
			errorRate = errors / requests * 100
		}

		latency := time.Duration(values["latency"] * float64(time.Second))

		summaries = append(summaries, ServiceMetrics{
			Service:        service,
			Requests:       requests,
			Errors:         errors,
			ErrorRate:      errorRate,
			AverageLatency: latency,
			InFlight:       values["in_flight"],
		})
	}

	return summaries, nil
}

func runMetricsSummary(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: lab metrics")
	}

	prometheusURL := os.Getenv("PROMETHEUS_URL")
	if prometheusURL == "" {
		prometheusURL = defaultPrometheusURL
	}

	client, err := newPrometheusClient(prometheusURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	summaries, err := client.summary(ctx)
	if err != nil {
		return err
	}

	printMetricsSummary(summaries)

	return nil
}

func printMetricsSummary(summaries []ServiceMetrics) {
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Service < summaries[j].Service
	})

	fmt.Println()
	fmt.Println("FailureLab — Metrics Summary")
	fmt.Println(strings.Repeat("─", 83))
	fmt.Printf(
		"%-12s %12s %10s %12s %15s %12s\n",
		"SERVICE",
		"REQUESTS",
		"ERRORS",
		"ERROR RATE",
		"AVG LATENCY",
		"IN FLIGHT",
	)

	var totalRequests float64
	var totalErrors float64

	for _, item := range summaries {
		totalRequests += item.Requests
		totalErrors += item.Errors

		fmt.Printf(
			"%-12s %12.0f %10.0f %11.2f%% %11.2f ms %12.0f\n",
			item.Service,
			item.Requests,
			item.Errors,
			item.ErrorRate,
			float64(item.AverageLatency)/float64(time.Millisecond),
			item.InFlight,
		)
	}

	totalErrorRate := float64(0)
	if totalRequests > 0 {
		totalErrorRate = totalErrors / totalRequests * 100
	}

	fmt.Println(strings.Repeat("─", 83))
	fmt.Printf(
		"%-12s %12.0f %10.0f %11.2f%%\n",
		"TOTAL",
		totalRequests,
		totalErrors,
		totalErrorRate,
	)
	fmt.Println()
}
