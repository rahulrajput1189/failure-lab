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
	"strings"
	"time"
)

const (
	defaultJaegerURL  = "http://localhost:16686"
	defaultLookback   = "1h"
	defaultTraceLimit = 20
)

type jaegerClient struct {
	baseURL    string
	httpClient *http.Client
}

type jaegerServicesResponse struct {
	Data []string `json:"data"`
}

type jaegerTracesResponse struct {
	Data []jaegerTrace `json:"data"`
}

type jaegerTrace struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
}

type jaegerSpan struct {
	TraceID       string      `json:"traceID"`
	SpanID        string      `json:"spanID"`
	OperationName string      `json:"operationName"`
	References    []any       `json:"references"`
	StartTime     int64       `json:"startTime"`
	Duration      int64       `json:"duration"`
	ProcessID     string      `json:"processID"`
	Tags          []jaegerTag `json:"tags"`
}

type jaegerProcess struct {
	ServiceName string `json:"serviceName"`
}

type jaegerTag struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type TraceSummary struct {
	TraceID   string
	Service   string
	Operation string
	Duration  time.Duration
	SpanCount int
	Services  []string
	StartTime int64
}

func newJaegerClient(baseURL string) (*jaegerClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Jaeger URL: %q", baseURL)
	}

	return &jaegerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (c *jaegerClient) get(ctx context.Context, endpoint string, target any) error {
	requestURL := c.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not connect to Jaeger: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf(
			"Jaeger returned HTTP %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("could not decode Jaeger response: %w", err)
	}

	return nil
}

func (c *jaegerClient) services(ctx context.Context) ([]string, error) {
	var response jaegerServicesResponse

	if err := c.get(ctx, "/api/services", &response); err != nil {
		return nil, fmt.Errorf("loading Jaeger services: %w", err)
	}

	services := make([]string, 0, len(response.Data))
	for _, service := range response.Data {
		service = strings.TrimSpace(service)
		if service == "" || service == "jaeger-all-in-one" {
			continue
		}
		services = append(services, service)
	}

	sort.Strings(services)
	return services, nil
}

func (c *jaegerClient) traces(
	ctx context.Context,
	service string,
	lookback string,
	limit int,
) ([]jaegerTrace, error) {
	params := url.Values{}
	params.Set("service", service)
	params.Set("lookback", lookback)
	params.Set("limit", fmt.Sprintf("%d", limit))

	endpoint := "/api/traces?" + params.Encode()

	var response jaegerTracesResponse
	if err := c.get(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("loading traces for %s: %w", service, err)
	}

	return response.Data, nil
}

func (c *jaegerClient) summary(
	ctx context.Context,
	lookback string,
	limit int,
) ([]TraceSummary, error) {
	services, err := c.services(ctx)
	if err != nil {
		return nil, err
	}

	tracesByID := make(map[string]jaegerTrace)

	for _, service := range services {
		traces, err := c.traces(ctx, service, lookback, limit)
		if err != nil {
			return nil, err
		}

		for _, trace := range traces {
			if trace.TraceID == "" {
				continue
			}
			tracesByID[trace.TraceID] = trace
		}
	}

	summaries := make([]TraceSummary, 0, len(tracesByID))

	for _, trace := range tracesByID {
		summary, ok := summarizeTrace(trace)
		if !ok {
			continue
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].StartTime > summaries[j].StartTime
	})

	return summaries, nil
}

func summarizeTrace(trace jaegerTrace) (TraceSummary, bool) {
	if len(trace.Spans) == 0 {
		return TraceSummary{}, false
	}

	var root *jaegerSpan

	for i := range trace.Spans {
		if len(trace.Spans[i].References) == 0 {
			root = &trace.Spans[i]
			break
		}
	}

	if root == nil {
		return TraceSummary{}, false
	}

	rootService := trace.Processes[root.ProcessID].ServiceName
	if rootService == "" {
		rootService = "unknown"
	}

	serviceSet := make(map[string]struct{})

	for _, span := range trace.Spans {
		process, exists := trace.Processes[span.ProcessID]
		if !exists || process.ServiceName == "" {
			continue
		}
		if process.ServiceName == "jaeger-all-in-one" {
			continue
		}
		serviceSet[process.ServiceName] = struct{}{}
	}

	participatingServices := make([]string, 0, len(serviceSet))
	for service := range serviceSet {
		participatingServices = append(participatingServices, service)
	}
	sort.Strings(participatingServices)

	return TraceSummary{
		TraceID:   trace.TraceID,
		Service:   rootService,
		Operation: rootOperation(*root),
		Duration:  time.Duration(root.Duration) * time.Microsecond,
		SpanCount: len(trace.Spans),
		Services:  participatingServices,
		StartTime: root.StartTime,
	}, true
}

func rootOperation(span jaegerSpan) string {
	method := tagString(span.Tags, "http.request.method")
	if method == "" {
		method = tagString(span.Tags, "http.method")
	}

	path := tagString(span.Tags, "url.path")
	if path == "" {
		path = tagString(span.Tags, "http.target")
	}

	if method != "" && path != "" {
		return method + " " + path
	}

	if method != "" && span.OperationName != "" &&
		span.OperationName != method &&
		!strings.HasPrefix(span.OperationName, method+" ") {
		return method + " " + span.OperationName
	}

	return span.OperationName
}

func tagString(tags []jaegerTag, key string) string {
	for _, tag := range tags {
		if tag.Key != key || tag.Value == nil {
			continue
		}

		switch value := tag.Value.(type) {
		case string:
			return value
		case float64:
			return fmt.Sprintf("%v", value)
		case bool:
			return fmt.Sprintf("%t", value)
		}
	}

	return ""
}

func runTracesSummary(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: lab traces")
	}

	jaegerURL := os.Getenv("JAEGER_QUERY_URL")
	if jaegerURL == "" {
		jaegerURL = defaultJaegerURL
	}

	client, err := newJaegerClient(jaegerURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	summaries, err := client.summary(ctx, defaultLookback, defaultTraceLimit)
	if err != nil {
		return err
	}

	printTraceSummary(summaries)
	return nil
}

func printTraceSummary(summaries []TraceSummary) {
	fmt.Println()
	fmt.Println("FailureLab — Distributed Traces")
	fmt.Printf("Lookback: %s | Limit per service: %d\n", defaultLookback, defaultTraceLimit)
	fmt.Println(strings.Repeat("─", 110))

	if len(summaries) == 0 {
		fmt.Println("No traces found.")
		fmt.Println()
		return
	}

	fmt.Printf(
		"%-12s %-22s %-24s %12s %8s %s\n",
		"ROOT SERVICE",
		"OPERATION",
		"TRACE ID",
		"DURATION",
		"SPANS",
		"SERVICES",
	)

	fmt.Println(strings.Repeat("─", 110))

	for _, item := range summaries {
		fmt.Printf(
			"%-12s %-22s %-24s %9.2f ms %8d %s\n",
			item.Service,
			truncate(item.Operation, 22),
			item.TraceID,
			float64(item.Duration)/float64(time.Millisecond),
			item.SpanCount,
			strings.Join(item.Services, ", "),
		)
	}

	fmt.Println(strings.Repeat("─", 110))
	fmt.Printf("Total traces: %d\n\n", len(summaries))
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
