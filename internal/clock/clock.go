// Package clock provides a time abstraction for testable time-dependent code.
package clock

import "time"

// Clock provides the current time.
// Use this interface in services that need to be tested with controlled time.
type Clock interface {
	Now() time.Time
}

// Real is a clock implementation that returns the actual current time.
type Real struct{}

// Now returns the current time from the system clock.
func (Real) Now() time.Time {
	return time.Now()
}

// New returns a new real clock.
func New() Clock {
	return Real{}
}
