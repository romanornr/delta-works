package contracts

// Logger defines the interface for logging.
type Logger interface {
	Info() LogEvent
	Debug() LogEvent
	Warn() LogEvent
	Error() LogEvent
}

// LogEvent defines the interface for a log event.
type LogEvent interface {
	Msg(msg string)
	Msgf(format string, a ...any)
	Err(err error) LogEvent
	Str(key, value string) LogEvent
	Int(key string, value int) LogEvent
}
