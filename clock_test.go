// Package openholidays — deterministic time/sleep test helper.
//
// This file implements D-95 verbatim: a tiny hand-rolled fakeClock used by
// downstream Phase 4 tests (retry_test.go for TEST-05, cache_test.go for
// TEST-06) to drive Client.nowFunc and Client.sleepFunc (added in Plan 02
// per D-94) without any wall-clock dependency.
//
// File placement convention mirrors transport_header_test.go::roundTripperFunc
// and transport_logging_test.go::trackedReader (D-50): a same-package _test.go
// file at repo root, so the helper is automatically excluded from `go build`
// yet visible to every other *_test.go in package openholidays without an
// import.
//
// Package declaration is `openholidays` (NOT `openholidays_test`) per D-94 —
// downstream test files need direct field access to Client.nowFunc and
// Client.sleepFunc, which are unexported.

package openholidays

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is a deterministic, race-safe clock used by tests that exercise
// timing-sensitive code (retry backoff, cache TTL eviction) without sleeping
// on the wall clock. Field order matches D-95 exactly.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock constructs a fakeClock seeded at the given instant. The
// returned pointer is safe to share across goroutines; concurrent calls to
// Now, Advance, and Sleep are serialized by an internal mutex.
func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

// Now returns the current fake time. Safe for concurrent use.
func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the fake clock forward by d. Safe for concurrent use.
func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Sleep advances the fake clock by d and returns nil, unless ctx is already
// cancelled, in which case it returns ctx.Err() immediately without
// advancing the clock. The signature `func(context.Context, time.Duration) error`
// matches Client.sleepFunc verbatim (D-94) so it can be assigned directly to
// that field by downstream tests without a conversion.
func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.Advance(d)
	return nil
}

// TestFakeClock_RaceFree is the self-verifying race-detector smoke test for
// the fakeClock helper. It exercises the three documented properties of
// fakeClock under `go test -race`:
//
//  1. Now and Advance are race-safe — 100 concurrent writers and 100
//     concurrent readers neither lose updates nor trip the race detector.
//  2. Sleep on a cancelled ctx returns ctx.Err() and does NOT advance.
//  3. Sleep on a live ctx advances by d and returns nil.
//
// Mirrors options_test.go etiquette: t.Parallel() at top and on every leaf
// subtest. One TestXxx per helper concern (Gold Rule 3 — the helper has one
// exported-style API surface from the consumer's perspective).
func TestFakeClock_RaceFree(t *testing.T) {
	t.Parallel()

	t.Run("Now and Advance are race-free under 100 goroutines", func(t *testing.T) {
		t.Parallel()
		start := time.Unix(0, 0)
		fc := newFakeClock(start)

		const writers = 100
		const writerIters = 100
		const readers = 100
		const readerIters = 100

		var wg sync.WaitGroup
		wg.Add(writers + readers)
		for range writers {
			go func() {
				defer wg.Done()
				for range writerIters {
					fc.Advance(time.Millisecond)
				}
			}()
		}
		for range readers {
			go func() {
				defer wg.Done()
				for range readerIters {
					_ = fc.Now()
				}
			}()
		}
		wg.Wait()

		want := start.Add(time.Duration(writers*writerIters) * time.Millisecond)
		assert.Equal(t, want, fc.Now(),
			"all %d Advance calls must be observed (no lost updates under contention)",
			writers*writerIters)
	})

	t.Run("Sleep returns ctx.Err() on cancelled ctx without advancing the clock", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		start := time.Unix(0, 0)
		fc := newFakeClock(start)

		err := fc.Sleep(ctx, time.Hour)
		require.ErrorIs(t, err, context.Canceled,
			"Sleep must return ctx.Err() when ctx is already cancelled")
		assert.Equal(t, start, fc.Now(),
			"Sleep on a cancelled ctx must NOT advance the fake clock")
	})

	t.Run("Sleep advances the clock by d and returns nil on live ctx", func(t *testing.T) {
		t.Parallel()
		start := time.Unix(0, 0)
		fc := newFakeClock(start)

		err := fc.Sleep(t.Context(), 5*time.Second)
		require.NoError(t, err,
			"Sleep on a live ctx must return nil")
		assert.Equal(t, start.Add(5*time.Second), fc.Now(),
			"Sleep on a live ctx must advance the fake clock by d")
	})
}
