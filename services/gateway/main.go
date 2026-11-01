package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"failure-lab/internal/metrics"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()

		r.Header.Set(requestIDHeader, requestID)

		w.Header().Del(requestIDHeader)
		slog.Info("request received",
			"service", "gateway",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		)
		next.ServeHTTP(w, r)

		w.Header().Set(requestIDHeader, requestID)
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

func liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"alive"}`)
}

func newOrdersProxy(targetURL string) (http.Handler, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("parse Orders service URL: %w", err)
	}

	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("invalid Orders service URL: %q", targetURL)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(target)
			req.SetXForwarded()
			req.Out.Header.Set(requestIDHeader, req.In.Header.Get(requestIDHeader))
		},
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy request failed",
			"service", "gateway",
			"target_service", "orders",
			"request_id", r.Header.Get(requestIDHeader),
			"error", err,
			"path", r.URL.Path,
			"method", r.Method,
		)
		http.Error(w, "orders service unavailable", http.StatusBadGateway)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/orders") {
			http.NotFound(w, r)
			return
		}

		proxy.ServeHTTP(w, r)
	}), nil
}

func newGatewayHandler(ordersServiceURL string) (http.Handler, error) {
	ordersProxy, err := newOrdersProxy(ordersServiceURL)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		health(w, r)
	})

	mux.HandleFunc("/liveness", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		liveness(w, r)
	})

	mux.Handle("/orders", ordersProxy)
	mux.Handle("/orders/", ordersProxy)

	serviceMetrics := metrics.New("gateway")
	mux.Handle("/metrics", serviceMetrics.Handler())

	handler := requestIDMiddleware(mux)

	handler = serviceMetrics.Middleware(metrics.RoutePattern, handler)

	return handler, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	config := loadConfig()

	handler, err := newGatewayHandler(config.OrdersServiceURL)
	if err != nil {
		slog.Error("failed to configure service",
			"service", "gateway",
			"error", err,
		)
		return
	}

	server := &http.Server{
		Addr:              ":" + config.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("service listening",
		"service", "gateway",
		"port", config.HTTPPort,
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP server failed",
			"service", "gateway",
			"error", err,
		)
	}
}
