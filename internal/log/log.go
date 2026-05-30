// Package log constructs the application's structured logger.
package log

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger that emits JSON to stderr at the given level.
// The level string is case-insensitive (debug|info|warn|error); an unknown or
// empty value falls back to info.
func New(level string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
