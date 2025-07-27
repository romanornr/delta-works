package container

import (
	"os"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/rs/zerolog"
)

// zerologLogger implements the Logger interface using zerolog
type zerologLogger struct {
	logger zerolog.Logger
}

// zerologEvent implements the LogEvent interface using zerolog
type zerologEvent struct {
	event *zerolog.Event
}

// NewDefaultLogger creates a new logger instance using zerolog.
func NewDefaultLogger() contracts.Logger {
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "2006-01-02 15:04:05",
		NoColor:    false,
	}

	logger := zerolog.New(output).With().Timestamp().Logger()

	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	return &zerologLogger{logger: logger}
}

// Info returns a new event with info level.
func (l *zerologLogger) Info() contracts.LogEvent {
	return &zerologEvent{event: l.logger.Info()}
}

// Debug returns a new event with debug level.
func (l *zerologLogger) Debug() contracts.LogEvent {
	return &zerologEvent{event: l.logger.Debug()}
}

// Warn returns a new event with warn level.
func (l *zerologLogger) Warn() contracts.LogEvent {
	return &zerologEvent{event: l.logger.Warn()}
}

// Error returns a new event with error level.
func (l *zerologLogger) Error() contracts.LogEvent {
	return &zerologEvent{event: l.logger.Error()}
}

// Msg logs a message.
func (e *zerologEvent) Msg(msg string) {
	e.event.Msg(msg)
}

// Msgf logs a formatted message.
func (e *zerologEvent) Msgf(format string, v ...interface{}) {
	e.event.Msgf(format, v...)
}

// Err adds the error to the log event and returns the event for chaining.
func (e *zerologEvent) Err(err error) contracts.LogEvent {
	e.event = e.event.Err(err)
	return e
}

// Str adds a string key-value pair to the log event and returns the event for chaining.
func (e *zerologEvent) Str(key, value string) contracts.LogEvent {
	e.event = e.event.Str(key, value)
	return e
}

// Int adds an int key-value pair to the log event and returns the event for chaining.
func (e *zerologEvent) Int(key string, value int) contracts.LogEvent {
	e.event = e.event.Int(key, value)
	return e
}
