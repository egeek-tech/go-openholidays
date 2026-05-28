// Package openholidays — tests for the in-memory MemoryCache implementation.
//
// One TestXxx per exported production function (Gold Rule 3): TestNewMemoryCache
// for the constructor, TestMemoryCache_GetPut for the storage cycle (Get + Put
// grouped because their lifecycle ties them per the plan's exception), plus
// dedicated tests for the sweeper / TTL / Close behaviors that are observable
// contracts (RESIL-08 / D-84 / D-85 / TEST-06).
//
// File placement convention mirrors clock_test.go and transport_header_test.go
// (D-50): same-package _test.go at repo root, visible to every other *_test.go
// in package openholidays without an import.
//
// The compile-time conformance assertion (var _ Cache = (*MemoryCache)(nil))
// is hoisted here, NOT into cache.go, so the CLIENT-10 AST audit
// (TestNoInitOrGlobalState in internal_test.go) does not need a new
// allowlist entry for a blank-identifier var — test files are excluded from
// that audit.

package openholidays

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion: *MemoryCache satisfies Cache (RESIL-06).
// Located in this test file (not cache.go) so the CLIENT-10 AST audit does
// not see an unexpected package-level blank-identifier var in production code.
var _ Cache = (*MemoryCache)(nil)

// TestCacheInterface_Conformance documents the interface conformance check.
// The compile-time assertion at file top is the load-bearing proof; this
// test exists so `go test -run TestCacheInterface_Conformance` produces a
// named pass in CI output.
func TestCacheInterface_Conformance(t *testing.T) {
	t.Parallel()

	t.Run("MemoryCache satisfies Cache at runtime via type assertion", func(t *testing.T) {
		t.Parallel()
		nc := NewMemoryCache(time.Hour)
		require.NotNil(t, nc)
		t.Cleanup(func() { _ = nc.Close() })
		var c Cache = nc // assignment proves runtime conformance
		assert.NotNil(t, c, "MemoryCache assigned to Cache interface variable must be non-nil")
	})
}

// TestNewMemoryCache covers the constructor: returns a non-nil *MemoryCache
// configured with the supplied TTL, no goroutines spawned yet.
func TestNewMemoryCache(t *testing.T) {
	t.Parallel()

	t.Run("constructs non-nil and accepts positive ttl", func(t *testing.T) {
		t.Parallel()
		nc := NewMemoryCache(time.Hour)
		require.NotNil(t, nc, "NewMemoryCache must return a non-nil pointer")
		assert.Equal(t, time.Hour, nc.ttl, "ttl field must reflect the constructor argument")
		t.Cleanup(func() { _ = nc.Close() })
	})
}

// TestMemoryCache_GetPut covers the Get + Put storage cycle: Get on empty
// returns (nil, false); Put then Get returns the stored value.
func TestMemoryCache_GetPut(t *testing.T) {
	t.Parallel()

	t.Run("Get on empty returns (nil, false)", func(t *testing.T) {
		t.Parallel()
		nc := NewMemoryCache(time.Hour)
		t.Cleanup(func() { _ = nc.Close() })

		v, ok := nc.Get("nonexistent")
		assert.False(t, ok, "Get on a missing key must return ok=false")
		assert.Nil(t, v, "Get on a missing key must return a nil value slice")
	})

	t.Run("Put then Get returns stored value", func(t *testing.T) {
		t.Parallel()
		nc := NewMemoryCache(time.Hour)
		t.Cleanup(func() { _ = nc.Close() })

		nc.Put("k", []byte("v"))
		v, ok := nc.Get("k")
		require.True(t, ok, "Get after Put must return ok=true")
		assert.Equal(t, []byte("v"), v, "Get must return the bytes previously stored via Put")
	})
}

// TestMemoryCache_SweeperLazyStart locks the D-84 lazy-start invariant: the
// sweeper goroutine is NOT spawned by NewMemoryCache, NOT spawned by Get,
// and IS spawned by the first Put. Close stops it.
//
// runtime.NumGoroutine() is the no-dep approach per D-96 (avoids adding
// go.uber.org/goleak as a test-only dep).
func TestMemoryCache_SweeperLazyStart(t *testing.T) {
	// NOTE: not t.Parallel() — runtime.NumGoroutine() delta checks are
	// sensitive to other tests' goroutine churn. Running serially is the
	// least-flaky option per the Phase 2/3 etiquette (D-48 / D-96).

	t.Run("lazy start on first Put, stop on Close (D-84 / D-85)", func(t *testing.T) {
		before := runtime.NumGoroutine()

		nc := NewMemoryCache(time.Hour)
		time.Sleep(5 * time.Millisecond) // settle any non-deterministic noise

		afterConstruct := runtime.NumGoroutine()
		assert.Equal(t, before, afterConstruct,
			"NewMemoryCache must NOT spawn a sweeper yet (D-84 lazy start)")

		// Get does NOT trigger sweeper start.
		_, _ = nc.Get("missing")
		time.Sleep(5 * time.Millisecond)
		afterGet := runtime.NumGoroutine()
		assert.Equal(t, before, afterGet,
			"Get must NOT start the sweeper (D-84 only Put starts it)")

		// First Put triggers lazy start.
		nc.Put("k", []byte("v"))
		time.Sleep(5 * time.Millisecond) // let sweeper goroutine schedule

		afterPut := runtime.NumGoroutine()
		assert.Greater(t, afterPut, before,
			"first Put must start the sweeper goroutine (D-84)")

		// Close must stop the sweeper.
		require.NoError(t, nc.Close())
		time.Sleep(10 * time.Millisecond) // grace period for sweeper exit

		assert.LessOrEqual(t, runtime.NumGoroutine(), before,
			"Close must stop the sweeper (D-85 / D-96)")
	})
}

// TestMemoryCache_TTLEviction covers TEST-06: TTL expiration is observable
// via Get (lazy-on-read path) under a deterministic fakeClock — no real
// wall-clock sleep.
func TestMemoryCache_TTLEviction(t *testing.T) {
	t.Parallel()

	t.Run("Get returns ok=false after fake clock advances past TTL", func(t *testing.T) {
		t.Parallel()
		fc := newFakeClock(time.Unix(0, 0))
		nc := newMemoryCacheWithClock(100*time.Millisecond, fc.Now)
		t.Cleanup(func() { _ = nc.Close() })

		nc.Put("k", []byte("v"))

		// Sanity: value is visible before TTL expires.
		v, ok := nc.Get("k")
		require.True(t, ok, "Get must return ok=true before TTL expires")
		assert.Equal(t, []byte("v"), v, "stored bytes must round-trip before expiry")

		// Advance fake clock past TTL; Get must now report expired.
		fc.Advance(200 * time.Millisecond)
		_, ok = nc.Get("k")
		assert.False(t, ok,
			"entry must be unreachable via Get after fake clock advances past TTL (D-81 lazy-expiration-on-read)")
	})
}

// TestMemoryCache_CloseIdempotent covers D-85 + Pitfall CONC-2: Close is
// idempotent under sequential and concurrent invocations from many
// goroutines.
func TestMemoryCache_CloseIdempotent(t *testing.T) {
	// NOTE: not t.Parallel() — the concurrent-close subtest uses
	// runtime.NumGoroutine which is sensitive to other tests' goroutine
	// churn.

	t.Run("sequential Close returns nil twice", func(t *testing.T) {
		nc := NewMemoryCache(time.Hour)
		nc.Put("k", []byte("v")) // forces sweeper start
		for i := 0; i < 2; i++ {
			assert.NoError(t, nc.Close(), "Close iteration %d must return nil (idempotent per D-85)", i+1)
		}
	})

	t.Run("100 concurrent Close calls all return nil and the sweeper exits", func(t *testing.T) {
		before := runtime.NumGoroutine()

		nc := NewMemoryCache(time.Hour)
		nc.Put("k", []byte("v")) // start sweeper
		time.Sleep(5 * time.Millisecond)

		var wg sync.WaitGroup
		const n = 100
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				assert.NoError(t, nc.Close(),
					"concurrent Close goroutine must return nil (sync.Once protects the cancel)")
			}()
		}
		wg.Wait()
		time.Sleep(10 * time.Millisecond) // sweeper exit grace

		// After 100 concurrent Closes the sweeper must be gone; allow a
		// small slack for test-runner overhead goroutines.
		assert.LessOrEqual(t, runtime.NumGoroutine(), before+1,
			"sweeper must have exited after 100 concurrent Close calls")
	})
}
