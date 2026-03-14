// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/daao/nexus/internal/database"
	"github.com/daao/nexus/internal/stream"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// Attribution for updates from cockpit
	cockpitAttribution = "user@cockpit"
)

// ============================================================================
// Request Types
// ============================================================================

// CreateContextFileRequest represents a request to create a new context file
type CreateContextFileRequest struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// UpdateContextFileRequest represents a request to update a context file
type UpdateContextFileRequest struct {
	Content string `json:"content"`
}

// ============================================================================
// Response Types
// ============================================================================

// ContextFileResponse represents a context file in API responses
type ContextFileResponse struct {
	ID             uuid.UUID `json:"id"`
	SatelliteID    uuid.UUID `json:"satellite_id"`
	FilePath       string    `json:"file_path"`
	Content        string    `json:"content"`
	Version        int       `json:"version"`
	LastModifiedBy string    `json:"last_modified_by"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
}

// ContextFileHistoryResponse represents a history entry in API responses
type ContextFileHistoryResponse struct {
	ID            uuid.UUID `json:"id"`
	ContextFileID uuid.UUID `json:"context_file_id"`
	Version       int       `json:"version"`
	Content       string    `json:"content"`
	ModifiedBy    string    `json:"modified_by"`
	ModifiedAt    string    `json:"modified_at"`
	Diff          string    `json:"diff,omitempty"`
}

// ContextFilesListResponse represents a list of context files
type ContextFilesListResponse struct {
	Files []ContextFileResponse `json:"files"`
	Count int                   `json:"count"`
}

// ContextFileHistoryListResponse represents a list of history entries
type ContextFileHistoryListResponse struct {
	History []ContextFileHistoryResponse `json:"history"`
	Count   int                          `json:"count"`
}

// ContextHandler provides HTTP handlers for context file CRUD operations.
type ContextHandler struct {
	dbPool         *pgxpool.Pool
	streamRegistry stream.StreamRegistryInterface
}

// NewContextHandler creates a new ContextHandler instance.
func NewContextHandler(pool *pgxpool.Pool, registry stream.StreamRegistryInterface) *ContextHandler {
	return &ContextHandler{
		dbPool:         pool,
		streamRegistry: registry,
	}
}

// RegisterContextRoutes registers context file routes to the given mux.
// NOTE: This no longer registers routes directly. Instead, the NexusServer
// registers a unified /api/v1/satellites/ handler that dispatches to both
// satellite CRUD (delete, rename, telemetry) and context file operations.
// The ContextHandler is accessed via its exported HandleContextAPI method.
func RegisterContextRoutes(mux *http.ServeMux, h *ContextHandler) {
	// Routes are registered by the NexusServer's handleSatellitesSubpath
	// to avoid intercepting satellite DELETE/PATCH requests.
}

// HandleContextAPI handles all context file API requests
func (h *ContextHandler) HandleContextAPI(w http.ResponseWriter, r *http.Request) {
	// Check if database is configured
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := strings.TrimRight(r.URL.Path, "/")

	// Parse: /api/v1/satellites/{satelliteID}/context
	//        /api/v1/satellites/{satelliteID}/context/{fileID}
	//        /api/v1/satellites/{satelliteID}/context/{fileID}/history

	if !strings.HasPrefix(p, "/api/v1/satellites/") {
		http.NotFound(w, r)
		return
	}

	// Extract satellite ID and remaining path
	remainder := strings.TrimPrefix(p, "/api/v1/satellites/")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	satelliteID := parts[0]
	satelliteUUID, err := uuid.Parse(satelliteID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid satellite ID")
		return
	}

	// Check if satellite exists
	if err := h.verifySatelliteExists(r.Context(), satelliteUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "satellite not found")
			return
		}
		slog.Info(fmt.Sprintf("verifySatelliteExists: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to verify satellite")
		return
	}

	// No context path - just satellite ID
	if len(parts) < 2 || parts[1] == "" {
		http.NotFound(w, r)
		return
	}

	contextPath := parts[1]

	// Route based on path structure
	switch {
	// POST /api/v1/satellites/{id}/context - create new context file
	case contextPath == "context" && r.Method == "POST":
		h.handleCreateContextFile(w, r, satelliteUUID)

	// GET /api/v1/satellites/{id}/context - list context files
	case contextPath == "context" && r.Method == "GET":
		h.handleListContextFiles(w, r, satelliteUUID)

	// DELETE /api/v1/satellites/{id}/context/{fileId} - delete context file
	case strings.HasPrefix(contextPath, "context/") && r.Method == "DELETE":
		fileID := strings.TrimPrefix(contextPath, "context/")
		h.handleDeleteContextFile(w, r, satelliteUUID, fileID)

	// GET /api/v1/satellites/{id}/context/{fileId} - get context file
	case strings.HasPrefix(contextPath, "context/") && r.Method == "GET":
		if strings.HasSuffix(contextPath, "/history") {
			// GET /api/v1/satellites/{id}/context/{fileId}/history
			fileID := strings.TrimSuffix(strings.TrimPrefix(contextPath, "context/"), "/history")
			h.handleGetContextFileHistory(w, r, satelliteUUID, fileID)
		} else {
			// GET /api/v1/satellites/{id}/context/{fileId}
			fileID := strings.TrimPrefix(contextPath, "context/")
			h.handleGetContextFile(w, r, satelliteUUID, fileID)
		}

	// PUT /api/v1/satellites/{id}/context/{fileId} - update context file
	case strings.HasPrefix(contextPath, "context/") && r.Method == "PUT":
		fileID := strings.TrimPrefix(contextPath, "context/")
		h.handleUpdateContextFile(w, r, satelliteUUID, fileID)

	default:
		http.NotFound(w, r)
	}
}

// verifySatelliteExists checks if a satellite exists in the database
func (h *ContextHandler) verifySatelliteExists(ctx context.Context, satelliteID uuid.UUID) error {
	var status string
	err := h.dbPool.QueryRow(ctx, "SELECT status FROM satellites WHERE id = $1", satelliteID).Scan(&status)
	return err
}

// handleListContextFiles handles GET /api/v1/satellites/:id/context
func (h *ContextHandler) handleListContextFiles(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID) {
	files, err := database.ListContextFiles(r.Context(), h.dbPool, satelliteID)
	if err != nil {
		slog.Info(fmt.Sprintf("ListContextFiles: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list context files")
		return
	}

	if files == nil {
		files = []database.ContextFile{}
	}

	response := ContextFilesListResponse{
		Files: make([]ContextFileResponse, len(files)),
		Count: len(files),
	}

	for i, f := range files {
		response.Files[i] = ContextFileResponse{
			ID:             f.ID,
			SatelliteID:    f.SatelliteID,
			FilePath:       f.FilePath,
			Content:        f.Content,
			Version:        f.Version,
			LastModifiedBy: f.LastModifiedBy,
			CreatedAt:      formatTime(f.CreatedAt),
			UpdatedAt:      formatTime(f.UpdatedAt),
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetContextFile handles GET /api/v1/satellites/:id/context/:fileId
func (h *ContextHandler) handleGetContextFile(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID, fileID string) {
	fileUUID, err := uuid.Parse(fileID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid file ID")
		return
	}

	// Verify the file belongs to this satellite
	file, err := database.GetContextFile(r.Context(), h.dbPool, fileUUID)
	if err != nil {
		slog.Info(fmt.Sprintf("GetContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get context file")
		return
	}

	if file == nil {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	// Ensure the file belongs to the specified satellite
	if file.SatelliteID != satelliteID {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	response := ContextFileResponse{
		ID:             file.ID,
		SatelliteID:    file.SatelliteID,
		FilePath:       file.FilePath,
		Content:        file.Content,
		Version:        file.Version,
		LastModifiedBy: file.LastModifiedBy,
		CreatedAt:      formatTime(file.CreatedAt),
		UpdatedAt:      formatTime(file.UpdatedAt),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleCreateContextFile handles POST /api/v1/satellites/:id/context
func (h *ContextHandler) handleCreateContextFile(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID) {
	var req CreateContextFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FilePath == "" {
		writeJSONError(w, http.StatusBadRequest, "file_path is required")
		return
	}

	// Create the context file with cockpit attribution
	file, err := database.CreateContextFile(r.Context(), h.dbPool, satelliteID, req.FilePath, req.Content, cockpitAttribution)
	if err != nil {
		slog.Info(fmt.Sprintf("CreateContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to create context file")
		return
	}

	response := ContextFileResponse{
		ID:             file.ID,
		SatelliteID:    file.SatelliteID,
		FilePath:       file.FilePath,
		Content:        file.Content,
		Version:        file.Version,
		LastModifiedBy: file.LastModifiedBy,
		CreatedAt:      formatTime(file.CreatedAt),
		UpdatedAt:      formatTime(file.UpdatedAt),
	}

	writeJSON(w, http.StatusCreated, response)
}

// handleUpdateContextFile handles PUT /api/v1/satellites/:id/context/:fileId
func (h *ContextHandler) handleUpdateContextFile(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID, fileID string) {
	fileUUID, err := uuid.Parse(fileID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid file ID")
		return
	}

	var req UpdateContextFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Verify the file exists and belongs to this satellite
	existing, err := database.GetContextFile(r.Context(), h.dbPool, fileUUID)
	if err != nil {
		slog.Info(fmt.Sprintf("GetContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get context file")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	if existing.SatelliteID != satelliteID {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	// Update with cockpit attribution
	file, err := database.UpdateContextFile(r.Context(), h.dbPool, fileUUID, req.Content, cockpitAttribution)
	if err != nil {
		slog.Info(fmt.Sprintf("UpdateContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to update context file")
		return
	}

	// Push the updated file to the satellite so the on-disk copy stays in sync.
	if h.streamRegistry != nil {
		push := &proto.NexusMessage{
			Payload: &proto.NexusMessage_ContextFilePush{
				ContextFilePush: &proto.ContextFilePush{
					File: &proto.ContextFileSync{
						SatelliteId: satelliteID.String(),
						FilePath:    file.FilePath,
						Content:     []byte(file.Content),
						Version:     int64(file.Version),
						ModifiedBy:  cockpitAttribution,
					},
				},
			},
		}
		if !h.streamRegistry.SendToSatellite(satelliteID.String(), push) {
			slog.Info(fmt.Sprintf("ContextFilePush: satellite %s not connected, file will sync on next reconnect", satelliteID), "component", "api")
		}
	}

	response := ContextFileResponse{
		ID:             file.ID,
		SatelliteID:    file.SatelliteID,
		FilePath:       file.FilePath,
		Content:        file.Content,
		Version:        file.Version,
		LastModifiedBy: file.LastModifiedBy,
		CreatedAt:      formatTime(file.CreatedAt),
		UpdatedAt:      formatTime(file.UpdatedAt),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleDeleteContextFile handles DELETE /api/v1/satellites/:id/context/:fileId
func (h *ContextHandler) handleDeleteContextFile(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID, fileID string) {
	fileUUID, err := uuid.Parse(fileID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid file ID")
		return
	}

	// Verify the file exists and belongs to this satellite
	existing, err := database.GetContextFile(r.Context(), h.dbPool, fileUUID)
	if err != nil {
		slog.Info(fmt.Sprintf("GetContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get context file")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	if existing.SatelliteID != satelliteID {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	if err := database.DeleteContextFile(r.Context(), h.dbPool, fileUUID); err != nil {
		slog.Info(fmt.Sprintf("DeleteContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to delete context file")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetContextFileHistory handles GET /api/v1/satellites/:id/context/:fileId/history
func (h *ContextHandler) handleGetContextFileHistory(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID, fileID string) {
	fileUUID, err := uuid.Parse(fileID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid file ID")
		return
	}

	// Verify the file exists and belongs to this satellite
	existing, err := database.GetContextFile(r.Context(), h.dbPool, fileUUID)
	if err != nil {
		slog.Info(fmt.Sprintf("GetContextFile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get context file")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	if existing.SatelliteID != satelliteID {
		writeJSONError(w, http.StatusNotFound, "context file not found")
		return
	}

	history, err := database.ListContextFileHistory(r.Context(), h.dbPool, fileUUID)
	if err != nil {
		slog.Info(fmt.Sprintf("ListContextFileHistory: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get context file history")
		return
	}

	if history == nil {
		history = []database.ContextFileHistory{}
	}

	// Get current content for diff calculation
	currentContent := existing.Content

	response := ContextFileHistoryListResponse{
		History: make([]ContextFileHistoryResponse, len(history)),
		Count:   len(history),
	}

	for i, hEntry := range history {
		// Calculate diff between this version and the next (or current for latest)
		var diffContent string
		if i == len(history)-1 {
			diffContent = currentContent
		} else {
			diffContent = history[i+1].Content
		}

		response.History[i] = ContextFileHistoryResponse{
			ID:            hEntry.ID,
			ContextFileID: hEntry.ContextFileID,
			Version:       hEntry.Version,
			Content:       hEntry.Content,
			ModifiedBy:    hEntry.ModifiedBy,
			ModifiedAt:    formatTime(hEntry.ModifiedAt),
			Diff:          computeDiff(hEntry.Content, diffContent),
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// formatTime handles time formatting from database (which may return different types)
func formatTime(t interface{}) string {
	if t == nil {
		return ""
	}
	switch v := t.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", t)
	}
}

// computeDiff computes a simple diff between two strings
func computeDiff(oldStr, newStr string) string {
	if oldStr == newStr {
		return ""
	}

	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	var result []string

	// Simple line-by-line diff
	oldIdx, newIdx := 0, 0

	for oldIdx < len(oldLines) || newIdx < len(newLines) {
		if oldIdx >= len(oldLines) {
			// Remaining new lines
			result = append(result, "+ "+newLines[newIdx])
			newIdx++
		} else if newIdx >= len(newLines) {
			// Remaining old lines
			result = append(result, "- "+oldLines[oldIdx])
			oldIdx++
		} else if oldLines[oldIdx] == newLines[newIdx] {
			// Same line
			result = append(result, "  "+oldLines[oldIdx])
			oldIdx++
			newIdx++
		} else {
			// Different lines - show both
			result = append(result, "- "+oldLines[oldIdx])
			result = append(result, "+ "+newLines[newIdx])
			oldIdx++
			newIdx++
		}
	}

	return strings.Join(result, "\n")
}
