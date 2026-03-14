// Package recording provides session recording and playback using asciicast v2 format.
//
// Recordings are stored as .cast files on disk (not in DB). The DB table
// session_recordings holds only metadata (id, session_id, filename, size, duration).
//
// Architecture:
//   - RecordingPool manages active recordings per session (map[sessionID]*RecordingWriter).
//   - RecordingWriter appends terminal output to a .cast file with timestamps.
//   - RecordingPlayer reads a .cast file and streams events for playback.
package recording

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// asciicastHeader is the first line of an asciicast v2 file.
type asciicastHeader struct {
	Version   int    `json:"version"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Title     string `json:"title,omitempty"`
}

// RecordingWriter writes terminal output to an asciicast v2 file.
// Thread-safe; designed for concurrent use from the gRPC gateway.
type RecordingWriter struct {
	mu          sync.Mutex
	file        *os.File
	recordingID string
	sessionID   string
	startTime   time.Time
	totalBytes  int64
	closed      bool
}

// NewRecordingWriter creates a new recording writer.
// It creates the .cast file and writes the asciicast v2 header.
func NewRecordingWriter(recordingID, sessionID, dataDir string, cols, rows int, title string) (*RecordingWriter, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recordings directory %s: %w", dataDir, err)
	}

	filename := filepath.Join(dataDir, recordingID+".cast")
	f, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create recording file %s: %w", filename, err)
	}

	now := time.Now()
	header := asciicastHeader{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: now.Unix(),
		Title:     title,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		f.Close()
		os.Remove(filename)
		return nil, fmt.Errorf("failed to marshal header: %w", err)
	}

	if _, err := f.Write(append(headerJSON, '\n')); err != nil {
		f.Close()
		os.Remove(filename)
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	return &RecordingWriter{
		file:        f,
		recordingID: recordingID,
		sessionID:   sessionID,
		startTime:   now,
	}, nil
}

// Write appends a terminal output event to the recording.
// Format: [elapsed_seconds, "o", "data"]\n
func (w *RecordingWriter) Write(data []byte) {
	if len(data) == 0 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	elapsed := time.Since(w.startTime).Seconds()

	// asciicast v2 event: [time, event_type, data]
	// We JSON-encode the data to escape special characters.
	dataStr, _ := json.Marshal(string(data))
	line := fmt.Sprintf("[%.6f, \"o\", %s]\n", elapsed, dataStr)

	if _, err := w.file.WriteString(line); err != nil {
		slog.Error(fmt.Sprintf("RecordingWriter[%s]: failed to write event: %v", w.sessionID, err), "component", "recording")
		return
	}

	w.totalBytes += int64(len(data))
}

// Close flushes and closes the recording file.
// Returns the duration in milliseconds and total bytes written.
func (w *RecordingWriter) Close() (durationMs int64, sizeBytes int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, 0
	}

	w.closed = true
	duration := time.Since(w.startTime)
	durationMs = duration.Milliseconds()
	sizeBytes = w.totalBytes

	if err := w.file.Sync(); err != nil {
		slog.Error(fmt.Sprintf("RecordingWriter[%s]: failed to sync: %v", w.sessionID, err), "component", "recording")
	}
	if err := w.file.Close(); err != nil {
		slog.Error(fmt.Sprintf("RecordingWriter[%s]: failed to close: %v", w.sessionID, err), "component", "recording")
	}

	slog.Info("recording closed", "component", "recording", "session_id", w.sessionID, "recording_id", w.recordingID, "duration_ms", durationMs, "size_bytes", sizeBytes)

	return durationMs, sizeBytes
}

// Filename returns the relative filename of the recording.
func (w *RecordingWriter) Filename() string {
	return w.recordingID + ".cast"
}

// RecordingID returns the recording ID.
func (w *RecordingWriter) RecordingID() string {
	return w.recordingID
}

// ──────────────────────────────────────────────────────────────────────────────
// RecordingPool manages active recordings per session.
// Only one active recording per session at a time.
// ──────────────────────────────────────────────────────────────────────────────

// RecordingPool manages active recording writers, keyed by session ID.
type RecordingPool struct {
	mu      sync.RWMutex
	writers map[string]*RecordingWriter
	dataDir string
}

// NewRecordingPool creates a new recording pool.
// dataDir is the base directory for recording files (e.g. /data/recordings).
func NewRecordingPool(dataDir string) *RecordingPool {
	return &RecordingPool{
		writers: make(map[string]*RecordingWriter),
		dataDir: dataDir,
	}
}

// StartRecording starts a new recording for a session.
// Returns the recording ID on success.
func (p *RecordingPool) StartRecording(sessionID, recordingID string, cols, rows int, title string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.writers[sessionID]; exists {
		return fmt.Errorf("session %s already has an active recording", sessionID)
	}

	w, err := NewRecordingWriter(recordingID, sessionID, p.dataDir, cols, rows, title)
	if err != nil {
		return err
	}

	p.writers[sessionID] = w
	slog.Info(fmt.Sprintf("RecordingPool: started recording %s for session %s", recordingID, sessionID), "component", "recording")
	return nil
}

// StopRecording stops the active recording for a session.
// Returns the duration in ms and size in bytes.
func (p *RecordingPool) StopRecording(sessionID string) (durationMs int64, sizeBytes int64, err error) {
	p.mu.Lock()
	w, exists := p.writers[sessionID]
	if exists {
		delete(p.writers, sessionID)
	}
	p.mu.Unlock()

	if !exists {
		return 0, 0, fmt.Errorf("no active recording for session %s", sessionID)
	}

	durationMs, sizeBytes = w.Close()
	slog.Info(fmt.Sprintf("RecordingPool: stopped recording for session %s", sessionID), "component", "recording")
	return durationMs, sizeBytes, nil
}

// WriteIfRecording writes data to the active recording for a session.
// This is a no-op if the session is not being recorded. Thread-safe.
func (p *RecordingPool) WriteIfRecording(sessionID string, data []byte) {
	p.mu.RLock()
	w, exists := p.writers[sessionID]
	p.mu.RUnlock()

	if exists && w != nil {
		w.Write(data)
	}
}

// IsRecording checks if a session has an active recording.
func (p *RecordingPool) IsRecording(sessionID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, exists := p.writers[sessionID]
	return exists
}

// StopAll stops all active recordings. Called during shutdown.
func (p *RecordingPool) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for sid, w := range p.writers {
		w.Close()
		slog.Info(fmt.Sprintf("RecordingPool: stopped recording for session %s (shutdown)", sid), "component", "recording")
	}
	p.writers = make(map[string]*RecordingWriter)
}

// DataDir returns the configured data directory.
func (p *RecordingPool) DataDir() string {
	return p.dataDir
}

// ReconcileRecordings scans the data directory for .cast files that are missing
// from the database and inserts metadata rows for them. This recovers recordings
// that were orphaned when their metadata was lost (e.g. from cascade deletes).
//
// Called on startup to ensure all on-disk recordings are discoverable.
func (p *RecordingPool) ReconcileRecordings(dbExec func(query string, args ...interface{}) error) {
	entries, err := os.ReadDir(p.dataDir)
	if err != nil {
		slog.Error(fmt.Sprintf("ReconcileRecordings: failed to read data dir %s: %v", p.dataDir, err), "component", "recording")
		return
	}

	reconciled := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".cast" {
			continue
		}

		recordingID := entry.Name()[:len(entry.Name())-5] // strip .cast
		filename := entry.Name()

		// Read .cast header to get dimensions and title
		filePath := filepath.Join(p.dataDir, filename)
		f, err := os.Open(filePath)
		if err != nil {
			continue
		}

		var header asciicastHeader
		decoder := json.NewDecoder(f)
		if err := decoder.Decode(&header); err != nil {
			f.Close()
			continue
		}
		f.Close()

		// Get file size
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Try to insert — ON CONFLICT DO NOTHING means we skip if already in DB
		err = dbExec(
			`INSERT INTO session_recordings (id, session_id, filename, size_bytes, duration_ms, started_at, created_at)
			 VALUES ($1, NULL, $2, $3, 0, to_timestamp($4), to_timestamp($4))
			 ON CONFLICT (id) DO NOTHING`,
			recordingID, filename, info.Size(), header.Timestamp,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("ReconcileRecordings: failed to insert %s: %v", filename, err), "component", "recording")
			continue
		}
		reconciled++
	}

	if reconciled > 0 {
		slog.Info(fmt.Sprintf("ReconcileRecordings: recovered %d orphaned recording(s)", reconciled), "component", "recording")
	}
}

// GetPlaybackURL returns empty string for local mode — caller serves via http.ServeFile.
func (p *RecordingPool) GetPlaybackURL(filename string) (string, error) {
	return "", nil
}

var _ RecordingPoolInterface = (*RecordingPool)(nil)
