package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mininext_http_requests_total",
		Help: "Total HTTP requests by method, path pattern, and status code.",
	}, []string{"method", "path", "status"})

	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mininext_http_request_duration_seconds",
		Help:    "HTTP request latency by method and path pattern.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	SnapshotBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mininext_snapshot_bytes",
		Help:    "Encoded snapshot size in bytes before compression.",
		Buckets: prometheus.ExponentialBuckets(1024, 2, 14), // 1KB–16MB
	}, []string{"format", "rendering"})

	SnapshotCompressedBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mininext_snapshot_compressed_bytes",
		Help:    "Compressed snapshot size in bytes.",
		Buckets: prometheus.ExponentialBuckets(512, 2, 14),
	}, []string{"format", "compression"})

	ActiveSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mininext_active_sessions",
		Help: "Number of currently active browser sessions.",
	})

	AdBlockBlocked = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mininext_adblock_requests_blocked_total",
		Help: "Total number of network requests blocked by the ad blocker.",
	})

	DeltaSnapshotsSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mininext_delta_snapshots_sent_total",
		Help: "Total number of delta (incremental) snapshots served.",
	})

	FullSnapshotsSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "mininext_full_snapshots_sent_total",
		Help: "Total number of full snapshots served.",
	})
)

func init() {
	prometheus.MustRegister(
		HTTPRequests,
		HTTPDuration,
		SnapshotBytes,
		SnapshotCompressedBytes,
		ActiveSessions,
		AdBlockBlocked,
		DeltaSnapshotsSent,
		FullSnapshotsSent,
	)
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// statusRecorder wraps ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware wraps an http.Handler with Prometheus instrumentation.
// pathPattern should be a stable pattern like "/api/v1/sessions/{id}/snapshot".
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		dur := time.Since(start).Seconds()

		path := sanitizePath(r.URL.Path)
		method := r.Method
		status := strconv.Itoa(rec.status)

		HTTPRequests.WithLabelValues(method, path, status).Inc()
		HTTPDuration.WithLabelValues(method, path).Observe(dur)
	})
}

// sanitizePath replaces UUIDs and numeric IDs in paths with placeholders
// so the cardinality of the path label stays bounded.
func sanitizePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		// UUID-like (32 hex chars, possibly with underscores prefix)
		if len(part) >= 32 && isHexLike(part) {
			parts[i] = "{id}"
			continue
		}
		// sess_/tab_/res_ prefixed IDs
		if strings.HasPrefix(part, "sess_") || strings.HasPrefix(part, "tab_") || strings.HasPrefix(part, "res_") {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func isHexLike(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '_') {
			return false
		}
	}
	return true
}
