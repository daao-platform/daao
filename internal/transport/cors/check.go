// Package cors provides CORS and origin validation utilities for WebSocket and WebTransport connections.
package cors

import (
	"net/http"
	"os"
	"strings"
)

// CheckOrigin validates the origin header for CORS compliance.
// It checks the DAAO_ALLOWED_ORIGINS env var first, with fallback to DAAO_CORS_ORIGINS for backward compatibility.
func CheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser clients (no Origin header)
	}
	// Allow configured origins via DAAO_ALLOWED_ORIGINS env var (with backward compat for DAAO_CORS_ORIGINS)
	allowedOrigins := os.Getenv("DAAO_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = os.Getenv("DAAO_CORS_ORIGINS") // backward compat
	}
	if allowedOrigins != "" {
		for _, allowed := range strings.Split(allowedOrigins, ",") {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
		return false
	}
	// Default: same-origin check — compare hostnames without port.
	// nginx proxies forward Host: without port ($host strips it), so
	// "localhost:8081" origin must match "localhost" request host.
	if strings.Contains(origin, "://") {
		parts := strings.SplitN(origin, "://", 2)
		if len(parts) == 2 {
			originHost := strings.Split(parts[1], ":")[0]
			requestHost := strings.Split(r.Host, ":")[0]
			return originHost == requestHost
		}
	}
	return false
}
