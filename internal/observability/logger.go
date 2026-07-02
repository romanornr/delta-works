package observability

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/rs/zerolog"
)

func NewLogger(cfg *config.Config) zerolog.Logger {
	var output io.Writer

	// Configure output format
	switch strings.ToLower(cfg.Logging.Format) {
	case "json":
		output = os.Stdout
	case "console":
		fallthrough
	default:
		output = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: cfg.Logging.TimeFormat,
		}
	}

	// Parse and set log level
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		// Invalid level string, default to info and log a warning.
		log.Printf("Warning: Invalid log level '%s' provided, defaulting to 'info'.\n", cfg.Logging.Level)
		level = zerolog.InfoLevel
	}

	// Build logger with standard fields
	logger := zerolog.New(output).Level(level).With().Timestamp().Str("app", cfg.App.Name).Logger()

	return logger
}
