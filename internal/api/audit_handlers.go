// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/daao/nexus/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditHandler provides HTTP handlers for audit log operations.
type AuditHandler struct {
	dbPool *pgxpool.Pool
}

// NewAuditHandler creates a new AuditHandler instance with dependencies injected.
func NewAuditHandler(pool *pgxpool.Pool) *AuditHandler {
	return &AuditHandler{
		dbPool: pool,
	}
}

// ============================================================================
// Response Types
// ============================================================================

// AuditLogResponse represents a paginated audit log response
type AuditLogResponse struct {
	Entries []AuditLogEntryResponse `json:"entries"`
	Total   int                      `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
}

// AuditLogEntryResponse represents an audit log entry in API responses
type AuditLogEntryResponse struct {
	ID           uuid.UUID   `json:"id"`
	ActorID      *uuid.UUID  `json:"actor_id"`
	ActorEmail   string      `json:"actor_email"`
	Action       string      `json:"action"`
	ResourceType string      `json:"resource_type"`
	ResourceID   *string     `json:"resource_id"`
	Details      interface{} `json:"details"`
	IPAddress    *string     `json:"ip_address"`
	CreatedAt    time.Time   `json:"created_at"`
}

// ============================================================================
// Handler Implementations
// ============================================================================

// HandleListAuditLog handles GET /api/v1/audit-log
// Query params: action, resource_type, actor_id, since, until, limit (default 50), offset (default 0)
// Response: { "entries": [...], "total": N, "limit": N, "offset": N }
func (h *AuditHandler) HandleListAuditLog(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse filters
	filters := database.AuditLogFilters{}

	if actorIDStr := r.URL.Query().Get("actor_id"); actorIDStr != "" {
		actorID, err := uuid.Parse(actorIDStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid actor_id")
			return
		}
		filters.ActorID = &actorID
	}

	if action := r.URL.Query().Get("action"); action != "" {
		filters.Action = &action
	}

	if resourceType := r.URL.Query().Get("resource_type"); resourceType != "" {
		filters.ResourceType = &resourceType
	}

	if since := r.URL.Query().Get("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid since parameter (use RFC3339 format)")
			return
		}
		filters.Since = &t
	}

	if until := r.URL.Query().Get("until"); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid until parameter (use RFC3339 format)")
			return
		}
		filters.Until = &t
	}

	// Parse pagination
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		if l > 500 {
			l = 500
		}
		limit = l
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		o, err := strconv.Atoi(offsetStr)
		if err != nil || o < 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid offset parameter")
			return
		}
		offset = o
	}

	// Fetch audit logs
	entries, total, err := database.ListAuditLogs(r.Context(), h.dbPool, filters, limit, offset)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListAuditLog: failed to list audit logs: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}

	if entries == nil {
		entries = []database.AuditLogEntry{}
	}

	// Convert to response format
	entryResponses := make([]AuditLogEntryResponse, len(entries))
	for i, entry := range entries {
		entryResponses[i] = AuditLogEntryResponse{
			ID:           entry.ID,
			ActorID:      entry.ActorID,
			ActorEmail:   entry.ActorEmail,
			Action:       entry.Action,
			ResourceType: entry.ResourceType,
			ResourceID:   entry.ResourceID,
			Details:      entry.Details,
			IPAddress:    entry.IPAddress,
			CreatedAt:    entry.CreatedAt,
		}
	}

	response := &AuditLogResponse{
		Entries: entryResponses,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}

	writeJSON(w, http.StatusOK, response)
}

// HandleExportAuditLog handles GET /api/v1/audit-log/export
// Query params: same filters as HandleListAuditLog, format=csv
// Response: text/csv with header row + data rows
func (h *AuditHandler) HandleExportAuditLog(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse filters
	filters := database.AuditLogFilters{}

	if actorIDStr := r.URL.Query().Get("actor_id"); actorIDStr != "" {
		actorID, err := uuid.Parse(actorIDStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid actor_id")
			return
		}
		filters.ActorID = &actorID
	}

	if action := r.URL.Query().Get("action"); action != "" {
		filters.Action = &action
	}

	if resourceType := r.URL.Query().Get("resource_type"); resourceType != "" {
		filters.ResourceType = &resourceType
	}

	if since := r.URL.Query().Get("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid since parameter (use RFC3339 format)")
			return
		}
		filters.Since = &t
	}

	if until := r.URL.Query().Get("until"); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid until parameter (use RFC3339 format)")
			return
		}
		filters.Until = &t
	}

	// Export with up to 10000 entries (no offset)
	limit := 10000
	offset := 0

	// Fetch audit logs
	entries, _, err := database.ListAuditLogs(r.Context(), h.dbPool, filters, limit, offset)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleExportAuditLog: failed to export audit logs: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to export audit logs")
		return
	}

	if entries == nil {
		entries = []database.AuditLogEntry{}
	}

	// Set headers for CSV
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=audit-log.csv")

	// Create CSV writer
	writer := csv.NewWriter(w)

	// Write header
	header := []string{"id", "actor_id", "actor_email", "action", "resource_type", "resource_id", "ip_address", "created_at", "details"}
	if err := writer.Write(header); err != nil {
		slog.Error(fmt.Sprintf("HandleExportAuditLog: failed to write CSV header: %v", err), "component", "api")
		return
	}

	// Write data rows
	for _, entry := range entries {
		var actorID, resourceID, ipAddress, details string

		if entry.ActorID != nil {
			actorID = entry.ActorID.String()
		}
		if entry.ResourceID != nil {
			resourceID = *entry.ResourceID
		}
		if entry.IPAddress != nil {
			ipAddress = *entry.IPAddress
		}
		if entry.Details != nil {
			detailsBytes, err := json.Marshal(entry.Details)
			if err == nil {
				details = string(detailsBytes)
			}
		}

		row := []string{
			entry.ID.String(),
			actorID,
			entry.ActorEmail,
			entry.Action,
			entry.ResourceType,
			resourceID,
			ipAddress,
			entry.CreatedAt.Format(time.RFC3339),
			details,
		}
		if err := writer.Write(row); err != nil {
			slog.Error(fmt.Sprintf("HandleExportAuditLog: failed to write CSV row: %v", err), "component", "api")
			return
		}
	}

	writer.Flush()
}

// handleAuditAPI handles all audit log API requests
func (h *AuditHandler) handleAuditAPI(w http.ResponseWriter, r *http.Request) {
	// Check if database is configured
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: GET /api/v1/audit-log - list audit logs
	if p == "/api/v1/audit-log" && method == "GET" {
		h.HandleListAuditLog(w, r)
		return
	}

	// Route: GET /api/v1/audit-log/export - export audit logs as CSV
	if p == "/api/v1/audit-log/export" && method == "GET" {
		h.HandleExportAuditLog(w, r)
		return
	}

	// 404 for unknown routes
	http.NotFound(w, r)
}

// RegisterAuditRoutes registers audit log API routes on the mux.
func RegisterAuditRoutes(mux *http.ServeMux, h *AuditHandler) {
	mux.HandleFunc("/api/v1/audit-log", h.handleAuditAPI)
	mux.HandleFunc("/api/v1/audit-log/", h.handleAuditAPI)
}
