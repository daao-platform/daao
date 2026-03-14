package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders_Present(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeadersMiddleware(inner)
	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := map[string]string{
		"Content-Security-Policy":   "default-src 'self'",
		"Strict-Transport-Security": "max-age=63072000",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=()",
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"X-Xss-Protection":          "0",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Cache-Control":             "no-store",
	}

	for header, wantSubstring := range expected {
		got := rec.Header().Get(header)
		if got == "" {
			t.Errorf("Missing security header: %s", header)
		} else if !strings.Contains(got, wantSubstring) {
			t.Errorf("Header %s = %q, want it to contain %q", header, got, wantSubstring)
		}
	}
}

func TestRequestBodyLimit_Enforced(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to read the full body
		buf := make([]byte, 2<<20) // 2 MB buffer
		_, err := r.Body.Read(buf)
		if err != nil && err.Error() == "http: request body too large" {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Limit to 1 KB for test
	handler := RequestBodyLimitMiddleware(1024)(inner)

	// Send a 2 KB body
	body := strings.Repeat("x", 2048)
	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected 413, got %d", rec.Code)
	}
}

func TestRequestBodyLimit_NormalSize(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 128)
		n, _ := r.Body.Read(buf)
		if n == 0 {
			t.Error("Expected to read body, got 0 bytes")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestBodyLimitMiddleware(1024)(inner)

	body := strings.Repeat("x", 100)
	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}
