// Package id generates ULIDs for identifiers we create, such as client
// order IDs and lot IDs. It is the only package that imports the ULID
// library; domain packages treat IDs as opaque strings and stay pure.
package id

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

var entropy = &ulid.LockedMonotonicReader{
	MonotonicReader: ulid.Monotonic(rand.Reader, 0),
}

// New returns a fresh ULID string: 26 characters, Crockford base32,
// lexicographically sortable by creation time at millisecond granularity.
// Safe for concurrent use. Without concurrent callers, successive IDs are
// strictly increasing even within one millisecond; concurrent callers can
// interleave the timestamp read and the entropy read, so ordering across
// goroutines holds only at the timestamp level.
func New() string {
	return ulid.MustNew(ulid.Timestamp(time.Now().UTC()), entropy).String()
}
