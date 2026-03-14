package recording

// RecordingPoolInterface abstracts local-disk and S3-backed recording pools.
// Satisfied by *RecordingPool (local) and *S3RecordingPool (enterprise).
type RecordingPoolInterface interface {
	StartRecording(sessionID, recordingID string, cols, rows int, title string) error
	StopRecording(sessionID string) (durationMs int64, sizeBytes int64, err error)
	WriteIfRecording(sessionID string, data []byte)
	IsRecording(sessionID string) bool
	StopAll()
	DataDir() string
	ReconcileRecordings(dbExec func(query string, args ...interface{}) error)
	// GetPlaybackURL returns an absolute URL for serving the recording.
	// Local mode returns ("", nil) — caller uses http.ServeFile from DataDir().
	// S3 mode returns a pre-signed URL — caller issues HTTP 302 redirect.
	GetPlaybackURL(filename string) (string, error)
}
