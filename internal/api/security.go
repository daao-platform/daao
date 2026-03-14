package api

import (
	"net/http"
)

// SecurityHeadersMiddleware adds security headers to all HTTP responses.
// These headers protect against common web vulnerabilities:
//   - CSP: prevents XSS by restricting resource loading
//   - HSTS: forces HTTPS connections
//   - Permissions-Policy: disables unnecessary browser features
//   - X-Frame-Options: prevents clickjacking
//   - X-Content-Type-Options: prevents MIME-type sniffing
//   - Referrer-Policy: limits referrer information leakage
//   - Cache-Control: prevents caching of sensitive API responses
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"connect-src 'self' wss: ws:; img-src 'self' data:; font-src 'self' data: https://fonts.gstatic.com")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// RequestBodyLimitMiddleware limits the size of request bodies to prevent
// denial-of-service attacks via oversized payloads. When the limit is
// exceeded, the server returns 413 Request Entity Too Large.
func RequestBodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
