// Package log is the single construction point for zerolog. Everything else
// imports this package and receives an injected Logger. Importing zerolog
// directly outside this package and cmd/ is denied by depguard.
package log

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/romanornr/delta-works/internal/config"
)

// Logger is the application logger type.
type Logger = zerolog.Logger

// New builds the root logger from configuration.
func New(cfg config.Log) (Logger, error) {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		return Logger{}, fmt.Errorf("parse log level %q: %w", cfg.Level, err)
	}

	var out io.Writer = os.Stderr
	if cfg.Format == "console" {
		out = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}
	}

	return zerolog.New(out).Level(level).With().Timestamp().Logger(), nil
}

// Component returns a child logger tagged with a component name.
func Component(l Logger, name string) Logger {
	return l.With().Str("component", name).Logger()
}
