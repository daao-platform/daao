package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// Recording Config (global toggle)
// ──────────────────────────────────────────────────────────────────────────────

// HandleGetRecordingConfig returns the global recording enabled flag.
// GET /api/v1/config/recording
func (h *Handlers) HandleGetRecordingConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	var value string
	err := h.dbPool.QueryRow(r.Context(),
		`SELECT value FROM app_settings WHERE key = 'recording_enabled'`,
	).Scan(&value)
	if err != nil {
		// Default to true if setting doesn't exist
		value = "true"
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recording_enabled": value == "true",
	})
}

// HandleSetRecordingConfig sets the global recording enabled flag.
// PUT /api/v1/config/recording
func (h *Handlers) HandleSetRecordingConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	var req struct {
		Enabled bool `json:"recording_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	value := "false"
	if req.Enabled {
		value = "true"
	}

	_, err := h.dbPool.Exec(r.Context(),
		`INSERT INTO app_settings (key, value, updated_at) VALUES ('recording_enabled', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW()`,
		value,
	)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleSetRecordingConfig: failed to update setting: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to update recording config"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recording_enabled": req.Enabled,
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Per-Session Recording Toggle
// ──────────────────────────────────────────────────────────────────────────────

// HandleToggleSessionRecording toggles recording_enabled on a session.
// PATCH /api/v1/sessions/{id}/recording
func (h *Handlers) HandleToggleSessionRecording(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	var req struct {
		Enabled bool `json:"recording_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	result, err := h.dbPool.Exec(r.Context(),
		`UPDATE sessions SET recording_enabled = $1, updated_at = NOW() WHERE id = $2 AND terminated_at IS NULL`,
		req.Enabled, id,
	)
	if err != nil {
		slog.Info(fmt.Sprintf("HandleToggleSessionRecording: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to update recording setting"})
		return
	}

	if result.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found or already terminated"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":                id.String(),
		"recording_enabled": req.Enabled,
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Start / Stop Recording
// ──────────────────────────────────────────────────────────────────────────────

// HandleStartRecording starts a new recording for a session.
// POST /api/v1/sessions/{id}/recording/start
func (h *Handlers) HandleStartRecording(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.recordingPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "recording not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	// Parse optional cols/rows from request body (allows frontend to pass actual terminal size)
	var req struct {
		Cols int `json:"cols,omitempty"`
		Rows int `json:"rows,omitempty"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req) // ignore errors — fields are optional
	}

	// Check session exists and get metadata
	sess, err := h.sessionStore.GetSession(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Check global recording is enabled
	if h.dbPool != nil {
		var globalEnabled string
		err := h.dbPool.QueryRow(r.Context(),
			`SELECT value FROM app_settings WHERE key = 'recording_enabled'`,
		).Scan(&globalEnabled)
		if err == nil && globalEnabled == "false" {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "recording is globally disabled"})
			return
		}
	}

	// Check per-session recording is enabled
	if !sess.RecordingEnabled {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "recording is disabled for this session"})
		return
	}

	// Check not already recording
	if h.recordingPool.IsRecording(sessionID) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session already has an active recording"})
		return
	}

	// Use frontend-provided dimensions if available, otherwise fall back to session DB values
	cols := int(sess.Cols)
	rows := int(sess.Rows)
	if req.Cols > 0 {
		cols = req.Cols
	}
	if req.Rows > 0 {
		rows = req.Rows
	}

	// Create recording
	recordingID := uuid.New().String()
	filename := recordingID + ".cast"

	if err := h.recordingPool.StartRecording(sessionID, recordingID, cols, rows, sess.Name); err != nil {
		slog.Info(fmt.Sprintf("HandleStartRecording: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to start recording"})
		return
	}

	// Persist metadata to DB
	if h.dbPool != nil {
		_, err := h.dbPool.Exec(r.Context(),
			`INSERT INTO session_recordings (id, session_id, filename, started_at)
			 VALUES ($1, $2, $3, NOW())`,
			recordingID, id, filename,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleStartRecording: failed to insert recording metadata: %v", err), "component", "api")
			// Don't fail — recording is active even if metadata write fails
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recording_id": recordingID,
		"session_id":   sessionID,
		"filename":     filename,
		"status":       "recording",
	})
}

// HandleStopRecording stops an active recording for a session.
// POST /api/v1/sessions/{id}/recording/stop
func (h *Handlers) HandleStopRecording(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.recordingPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "recording not configured"})
		return
	}

	if _, err := uuid.Parse(sessionID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	durationMs, sizeBytes, err := h.recordingPool.StopRecording(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	// Update metadata in DB
	if h.dbPool != nil {
		_, err := h.dbPool.Exec(r.Context(),
			`UPDATE session_recordings
			 SET stopped_at = NOW(), duration_ms = $1, size_bytes = $2
			 WHERE session_id = $3 AND stopped_at IS NULL`,
			durationMs, sizeBytes, sessionID,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleStopRecording: failed to update recording metadata: %v", err), "component", "api")
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id":  sessionID,
		"duration_ms": durationMs,
		"size_bytes":  sizeBytes,
		"status":      "stopped",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// List / Get / Stream Recordings
// ──────────────────────────────────────────────────────────────────────────────

// HandleListRecordings lists recordings for a session.
// GET /api/v1/sessions/{id}/recordings
func (h *Handlers) HandleListRecordings(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}

	if _, err := uuid.Parse(sessionID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	type RecordingRow struct {
		ID         string     `json:"id"`
		SessionID  string     `json:"session_id"`
		Filename   string     `json:"filename"`
		SizeBytes  int64      `json:"size_bytes"`
		DurationMs int64      `json:"duration_ms"`
		StartedAt  time.Time  `json:"started_at"`
		StoppedAt  *time.Time `json:"stopped_at"`
		CreatedAt  time.Time  `json:"created_at"`
	}

	rows, err := h.rpool().Query(r.Context(),
		`SELECT id::text, session_id::text, filename, size_bytes, duration_ms, started_at, stopped_at, created_at
		 FROM session_recordings WHERE session_id = $1 ORDER BY created_at DESC`,
		sessionID,
	)
	if err != nil {
		slog.Info(fmt.Sprintf("HandleListRecordings: %v", err), "component", "api")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()

	var recordings []RecordingRow
	for rows.Next() {
		var rec RecordingRow
		if err := rows.Scan(&rec.ID, &rec.SessionID, &rec.Filename, &rec.SizeBytes, &rec.DurationMs, &rec.StartedAt, &rec.StoppedAt, &rec.CreatedAt); err != nil {
			continue
		}
		recordings = append(recordings, rec)
	}

	if recordings == nil {
		recordings = []RecordingRow{}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(recordings)
}

// HandleListAllRecordings lists all recordings across all sessions.
// GET /api/v1/recordings
func (h *Handlers) HandleListAllRecordings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}

	type AllRecordingRow struct {
		ID          string     `json:"id"`
		SessionID   string     `json:"session_id"`
		SessionName string     `json:"session_name"`
		Filename    string     `json:"filename"`
		SizeBytes   int64      `json:"size_bytes"`
		DurationMs  int64      `json:"duration_ms"`
		StartedAt   time.Time  `json:"started_at"`
		StoppedAt   *time.Time `json:"stopped_at"`
		CreatedAt   time.Time  `json:"created_at"`
	}

	rows, err := h.rpool().Query(r.Context(),
		`SELECT r.id::text, COALESCE(r.session_id::text, ''), COALESCE(s.name, ''), r.filename,
		        r.size_bytes, r.duration_ms, r.started_at, r.stopped_at, r.created_at
		 FROM session_recordings r
		 LEFT JOIN sessions s ON s.id = r.session_id
		 ORDER BY r.created_at DESC
		 LIMIT 100`,
	)
	if err != nil {
		slog.Info(fmt.Sprintf("HandleListAllRecordings: %v", err), "component", "api")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()

	var recordings []AllRecordingRow
	for rows.Next() {
		var rec AllRecordingRow
		if err := rows.Scan(&rec.ID, &rec.SessionID, &rec.SessionName, &rec.Filename,
			&rec.SizeBytes, &rec.DurationMs, &rec.StartedAt, &rec.StoppedAt, &rec.CreatedAt); err != nil {
			continue
		}
		recordings = append(recordings, rec)
	}

	if recordings == nil {
		recordings = []AllRecordingRow{}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(recordings)
}

// HandleGetRecording gets metadata for a specific recording.
// GET /api/v1/recordings/{id}
func (h *Handlers) HandleGetRecording(w http.ResponseWriter, r *http.Request, recordingID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	var rec struct {
		ID         string     `json:"id"`
		SessionID  string     `json:"session_id"`
		Filename   string     `json:"filename"`
		SizeBytes  int64      `json:"size_bytes"`
		DurationMs int64      `json:"duration_ms"`
		StartedAt  time.Time  `json:"started_at"`
		StoppedAt  *time.Time `json:"stopped_at"`
		CreatedAt  time.Time  `json:"created_at"`
	}

	err := h.rpool().QueryRow(r.Context(),
		`SELECT id::text, COALESCE(session_id::text, ''), filename, size_bytes, duration_ms, started_at, stopped_at, created_at
		 FROM session_recordings WHERE id = $1::uuid`,
		recordingID,
	).Scan(&rec.ID, &rec.SessionID, &rec.Filename, &rec.SizeBytes, &rec.DurationMs, &rec.StartedAt, &rec.StoppedAt, &rec.CreatedAt)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "recording not found"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(rec)
}

// HandleStreamRecording streams a recording for playback via SSE (Server-Sent Events).
// GET /api/v1/recordings/{id}/stream?speed=1
func (h *Handlers) HandleStreamRecording(w http.ResponseWriter, r *http.Request, recordingID string) {
	if h.dbPool == nil || h.recordingPool == nil {
		http.Error(w, "recording not configured", http.StatusServiceUnavailable)
		return
	}

	// Look up recording metadata
	var filename string
	err := h.rpool().QueryRow(r.Context(),
		`SELECT filename FROM session_recordings WHERE id = $1::uuid`,
		recordingID,
	).Scan(&filename)
	if err != nil {
		http.Error(w, "recording not found", http.StatusNotFound)
		return
	}

	// Try S3 pre-signed URL first
	if playbackURL, err := h.recordingPool.GetPlaybackURL(filename); err != nil {
		slog.Warn("recording: GetPlaybackURL failed", "filename", filename, "err", err, "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "recording unavailable")
		return
	} else if playbackURL != "" {
		// S3 mode: redirect to pre-signed URL
		http.Redirect(w, r, playbackURL, http.StatusFound)
		return
	}
	// Local mode: serve from disk
	filePath := filepath.Join(h.recordingPool.DataDir(), filename)
	w.Header().Set("Content-Type", "application/x-asciicast")
	w.Header().Set("Content-Disposition", "inline; filename=\""+filename+"\"")
	http.ServeFile(w, r, filePath)
}
