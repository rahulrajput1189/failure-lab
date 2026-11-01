package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry         *prometheus.Registry
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestsInFlight prometheus.Gauge
}

func New(service string) *Metrics {
	registry := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "Total number of HTTP requests handled.",
			ConstLabels: prometheus.Labels{
				"service": service,
			},
		},
		[]string{"method", "route", "status"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_server_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
			ConstLabels: prometheus.Labels{
				"service": service,
			},
		},
		[]string{"method", "route"},
	)

	requestsInFlight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_server_requests_in_flight",
			Help: "Number of HTTP requests currently being handled.",
			ConstLabels: prometheus.Labels{
				"service": service,
			},
		},
	)

	registry.MustRegister(requestsTotal, requestDuration, requestsInFlight)

	return &Metrics{
		registry:         registry,
		requestsTotal:    requestsTotal,
		requestDuration:  requestDuration,
		requestsInFlight: requestsInFlight,
	}
}

func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Middleware(routePattern func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		m.requestsInFlight.Inc()
		defer m.requestsInFlight.Dec()

		recorder := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(recorder, r)

		route := "unmatched"
		if routePattern != nil {
			if pattern := routePattern(r); pattern != "" {
				route = pattern
			}
		}

		m.requestsTotal.WithLabelValues(
			r.Method,
			route,
			strconv.Itoa(recorder.status),
		).Inc()

		m.requestDuration.WithLabelValues(
			r.Method,
			route,
		).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}

	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	return r.ResponseWriter.Write(data)
}

func RoutePattern(r *http.Request) string {
	if r.URL.Path == "/metrics" {
		return "/metrics"
	}

	if r.Pattern != "" {
		return r.Pattern
	}

	switch {
	case r.URL.Path == "/health":
		return "/health"
	case r.URL.Path == "/liveness":
		return "/liveness"
	case r.URL.Path == "/orders":
		return "/orders"
	case strings.HasPrefix(r.URL.Path, "/orders/"):
		return "/orders/"
	case strings.HasPrefix(r.URL.Path, "/products/"):
		return "/products/"
	case r.URL.Path == "/payments":
		return "/payments"
	case strings.HasPrefix(r.URL.Path, "/payments/"):
		return "/payments/"
	default:
		return "unmatched"
	}
}
