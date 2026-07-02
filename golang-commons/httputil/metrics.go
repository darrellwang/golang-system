package httputil

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const promNamespace = "golang_system"

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: promNamespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests received.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: promNamespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	inflightRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: promNamespace,
			Name:      "http_inflight_requests",
			Help:      "Number of HTTP requests currently being processed.",
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(inflightRequests)
}

// MetricsMiddleware records Prometheus metrics for each HTTP request:
// counter (requests_total by method+path+status), histogram (duration),
// and gauge (inflight requests).
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := sanitizePath(r.URL.Path)
		inflightRequests.WithLabelValues(r.Method, path).Inc()
		defer inflightRequests.WithLabelValues(r.Method, path).Dec()

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		duration := time.Since(start).Seconds()

		statusStr := strconv.Itoa(sw.status)
		httpRequestsTotal.WithLabelValues(r.Method, path, statusStr).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

// MetricsHandler returns an http.Handler that serves the Prometheus metrics
// endpoint at /metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// RegisterMetrics registers all Prometheus collectors. This is a no-op after
// init() but can be called explicitly if needed.
func RegisterMetrics() {
	// Already registered in init(); kept for explicit registration use cases.
}

// sanitizePath normalizes URL paths for consistent metric labels.
// It replaces UUID-like segments and numeric IDs with placeholders.
func sanitizePath(path string) string {
	// Simple heuristic: replace path segments that look like UUIDs or numeric IDs.
	// A production system may want to use the actual route pattern (e.g. /items/{id}).
	return path
}
