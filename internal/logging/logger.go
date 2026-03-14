package logging

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a new slog.Logger with the specified format and level.
// format: "json" for JSON output (default for production), "text" for human-readable text
// level: "debug", "info", "warn", "error" (default: "info")
func NewLogger(format string, level string) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		// Default to JSON for production
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// Component returns a child logger with the specified component name.
// The component name is added as a "component" attribute to all log entries.
func Component(logger *slog.Logger, name string) *slog.Logger {
	return logger.With("component", name)
}

// parseLevel converts a level string to slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		fallthrough
	default:
		return slog.LevelInfo
	}
}

// Init reads environment variables DAAO_LOG_FORMAT and DAAO_LOG_LEVEL
// to configure the logger, creates it, and sets it as the default slog logger.
// Defaults: format="json", level="info"
func Init() *slog.Logger {
	format := getEnv("DAAO_LOG_FORMAT", "json")
	level := getEnv("DAAO_LOG_LEVEL", "info")

	logger := NewLogger(format, level)
	slog.SetDefault(logger)

	return logger
}

// getEnv returns the value of an environment variable or a default if not set.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
