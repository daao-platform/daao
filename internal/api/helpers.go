// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes a JSON response with the given status code.
// This is the single canonical JSON response helper for the entire api package.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeJSONError writes a JSON error response with the given status code and message.
// The response body is {"error": "<message>"}.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
