package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"os"
)

// log is the global logger instance.
var log zerolog.Logger

// timeFormat defines the standard format for timestamps: "2006-01-02 15:04:05".
const timeFormat = "2006-01-02 15:04:05"

// Init initializes the logger by setting the time format, error stack marshaler,
// output configuration, and default context logger. It uses the zerolog library
// to create a new logger with a console writer configured with the specified time
// format, color, and output stream. The logger includes a timestamp in the messages
// and is set as the default context logger for the zerolog library.
func Init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: timeFormat, NoColor: false}

	log = zerolog.New(output).With().Timestamp().Logger()

	zerolog.DefaultContextLogger = &log
	setLogLevel()
}

// setLogLevel sets the global log level based on the LOG_LEVEL environment variable.
func setLogLevel() {
	env := os.Getenv("LOG_LEVEL")
	switch env {
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
}

// GetLogger returns the global logger instance.
func GetLogger() zerolog.Logger {
	return log
}

// Debug returns a new event with debug level.
func Debug() *zerolog.Event {
	return log.Debug()
}

// Info returns a new event with info level.
func Info() *zerolog.Event {
	return log.Info()
}

// Warn returns a new event with warn level.
func Warn() *zerolog.Event {
	return log.Warn()
}

// Error returns a new event with error level.
func Error() *zerolog.Event {
	return log.Error()
}

// Fatal returns a new event with fatal level.
func Fatal() *zerolog.Event {
	return log.Fatal()
}
