// Package logger provides a structured, levelled logger built on zerolog.
// All log entries are JSON-formatted in production, making them easy to
// ingest into centralised logging systems (e.g. Datadog, CloudWatch).
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// New configures and returns a zerolog.Logger.  Pass the result to
// downstream components so the root logger is fully injectable.
//
//   - level: "debug" | "info" | "warn" | "error"
//   - format: "json" | "console" (console is human-readable, useful locally)
func New(level, format string) zerolog.Logger {
	// Map log level string → zerolog.Level
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	// Default to stdout; use a timestamp with millisecond precision
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var output io.Writer = os.Stdout
	if format == "console" {
		// Pretty-print for local development
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	logger := zerolog.New(output).With().
		Timestamp().
		Caller().
		Logger()

	// Replace the global logger so zerolog.Ctx helper works anywhere
	log.Logger = logger

	return logger
}
