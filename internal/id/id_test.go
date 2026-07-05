package id_test

import (
	"sync"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/romanornr/delta-works/internal/id"
)

// No test in this file runs parallel: strict ordering of successive IDs
// only holds without concurrent callers on the shared entropy source.
func TestNewParses(t *testing.T) {
	got := id.New()
	if len(got) != ulid.EncodedSize {
		t.Fatalf("len(New()) = %d, want %d", len(got), ulid.EncodedSize)
	}
	if _, err := ulid.ParseStrict(got); err != nil {
		t.Fatalf("ParseStrict(%q): %v", got, err)
	}
}

func TestNewSortedAndUnique(t *testing.T) {
	const n = 10_000
	prev := ""
	seen := make(map[string]struct{}, n)
	for range n {
		got := id.New()
		if got <= prev {
			t.Fatalf("IDs not strictly increasing: %q then %q", prev, got)
		}
		if _, dup := seen[got]; dup {
			t.Fatalf("duplicate ID %q", got)
		}
		seen[got] = struct{}{}
		prev = got
	}
}

func TestNewConcurrent(t *testing.T) {
	const goroutines, perGoroutine = 8, 1_000
	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range perGoroutine {
				got := id.New()
				mu.Lock()
				_, dup := seen[got]
				seen[got] = struct{}{}
				mu.Unlock()
				if dup {
					t.Errorf("duplicate ID %q", got)
					return
				}
			}
		})
	}
	wg.Wait()
}
