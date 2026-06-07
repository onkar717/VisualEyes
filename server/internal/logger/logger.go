// Package logger provides a structured slog-based logger for the VisualEyes server.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the global slog logger with the given level and format.
func Init(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// With returns a logger with the given key-value pairs pre-attached.
func With(args ...any) *slog.Logger {
	return slog.Default().With(args...)
}
