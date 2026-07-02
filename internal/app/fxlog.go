package app

import (
	"go.uber.org/fx/fxevent"

	"github.com/romanornr/delta-works/internal/log"
)

// fxLogger routes fx lifecycle events to zerolog: errors at error level,
// everything else at trace so normal operation stays quiet.
type fxLogger struct {
	l log.Logger
}

func newFxLogger(l log.Logger) fxevent.Logger {
	return &fxLogger{l: log.Component(l, "fx")}
}

func (f *fxLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.OnStartExecuted:
		if e.Err != nil {
			f.l.Error().Err(e.Err).Str("callee", e.FunctionName).Msg("OnStart hook failed")
		}
	case *fxevent.OnStopExecuted:
		if e.Err != nil {
			f.l.Error().Err(e.Err).Str("callee", e.FunctionName).Msg("OnStop hook failed")
		}
	case *fxevent.Provided:
		if e.Err != nil {
			f.l.Error().Err(e.Err).Str("constructor", e.ConstructorName).Msg("provide failed")
		}
	case *fxevent.Invoked:
		if e.Err != nil {
			f.l.Error().Err(e.Err).Str("function", e.FunctionName).Msg("invoke failed")
		}
	case *fxevent.Started:
		if e.Err != nil {
			f.l.Error().Err(e.Err).Msg("start failed")
		} else {
			f.l.Debug().Msg("started")
		}
	case *fxevent.Stopped:
		if e.Err != nil {
			f.l.Error().Err(e.Err).Msg("stop failed")
		} else {
			f.l.Debug().Msg("stopped")
		}
	}
}
