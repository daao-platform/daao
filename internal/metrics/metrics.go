package metrics

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metric names
const (
	SatellitesConnected   = "daao_satellites_connected"
	SessionsActive       = "daao_sessions_active"
	GRPCStreamsActive    = "daao_grpc_streams_active"
	HTTPRequestsTotal    = "daao_http_requests_total"
	HTTPRequestDuration  = "daao_http_request_duration_seconds"
	AgentRunsTotal       = "daao_agent_runs_total"
)

// Metrics
var (
	satellitesConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: SatellitesConnected,
		Help: "Number of satellites currently connected",
	})

	sessionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: SessionsActive,
		Help: "Number of active sessions",
	})

	grpcStreamsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: GRPCStreamsActive,
		Help: "Number of active gRPC streams",
	})

	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: HTTPRequestsTotal,
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    HTTPRequestDuration,
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	agentRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: AgentRunsTotal,
		Help: "Total number of agent runs",
	}, []string{"status"})
)

// Path normalization regex patterns to avoid cardinality explosion
var (
	uuidRegex       = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	numericRegex    = regexp.MustCompile(`/[-]?[0-9]+`)
	alphanumericRegex = regexp.MustCompile(`/[-a-zA-Z0-9]{20,}`)
)

// normalizePath converts a path to a normalized form to prevent cardinality explosion
// Examples:
//   - /api/v1/sessions/123e4567-e89b-12d3-a456-426614174000 -> /api/v1/sessions/:id
//   - /api/v1/users/123 -> /api/v1/users/:id
func normalizePath(path string) string {
	// Replace UUIDs with :id
	normalized := uuidRegex.ReplaceAllString(path, ":id")

	// Replace numeric IDs with :id
	normalized = numericRegex.ReplaceAllString(normalized, "/:id")

	// Replace long alphanumeric strings (like tokens) with :token
	normalized = alphanumericRegex.ReplaceAllString(normalized, "/:token")

	// If path is still too long or has many variables, simplify
	if len(normalized) > 50 {
		// Try to keep only the first few segments
		segments := splitPath(normalized)
		if len(segments) > 4 {
			normalized = joinPath(segments[:4]) + "/:rest"
		}
	}

	return normalized
}

func splitPath(path string) []string {
	if path == "" {
		return []string{}
	}
	// Remove leading slash
	if path[0] == '/' {
		path = path[1:]
	}
	if path == "" {
		return []string{}
	}
	result := []string{}
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func joinPath(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	result := segments[0]
	for i := 1; i < len(segments); i++ {
		result += "/" + segments[i]
	}
	return result
}

// responseWriterWrapper wraps http.ResponseWriter to capture status code.
// It also implements http.Hijacker and http.Flusher so that WebSocket
// upgrades and SSE streams work through the middleware chain.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseWriterWrapper) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker — required for WebSocket upgrade.
func (r *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// Flush implements http.Flusher — required for SSE (Server-Sent Events).
func (r *responseWriterWrapper) Flush() {
	if fl, ok := r.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

// MetricsMiddleware returns a middleware that records HTTP request metrics
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapper := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default to 200 in case WriteHeader is not called
		}

		// Process request
		next.ServeHTTP(wrapper, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		method := r.Method
		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(wrapper.statusCode)

		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpRequestDuration.WithLabelValues(method, path).Observe(duration)
	})
}

// Handler returns a Prometheus HTTP handler for the /metrics endpoint
func Handler() http.Handler {
	return promhttp.Handler()
}

// SetSatellitesConnected sets the number of connected satellites
func SetSatellitesConnected(n float64) {
	satellitesConnected.Set(n)
}

// SetSessionsActive sets the number of active sessions
func SetSessionsActive(n float64) {
	sessionsActive.Set(n)
}

// SetGRPCStreamsActive sets the number of active gRPC streams
func SetGRPCStreamsActive(n float64) {
	grpcStreamsActive.Set(n)
}

// IncAgentRun increments the agent runs counter for the given status
func IncAgentRun(status string) {
	agentRunsTotal.WithLabelValues(status).Inc()
}
