// in-memory TTL cache backing WithCache and the
// Cache interface.
//
// This file ships the default in-memory cache implementation that
// Client.WithCache enables (Plan 04 / RESIL-06..09 / D-79..D-86). The Cache
// interface itself is declared in config.go (Plan 02 D-79) so that
// Client.Close can call cache.Close without a build error before the
// implementation lands.
//
// Design (verbatim per .planning/phases/04-resilience/04-RESEARCH.md
// "Pattern 4: In-memory TTL cache"):
//
//   - Backing storage: map[string]entry under sync.RWMutex (Pitfall CACHE-4
//     — RWMutex over sync.Map because the workload is read-heavy with
//     occasional re-writes on TTL refresh; sync.Map's optimization targets
//     a different access pattern).
//   - Lazy sweeper start (D-84): the eviction goroutine is spawned on the
//     FIRST Put, never sooner — a constructed-but-unused MemoryCache costs
//     zero goroutines until it stores anything.
//   - Idempotent Close (D-85): a sync.Once guards the cancel + done-wait so
//     concurrent Close calls from any number of goroutines all return nil.
//   - Lazy expiration on read (D-81): Get checks expiresAt against nowFn
//     and reports a miss for stale entries without waiting for the
//     sweeper. The active sweeper exists to bound memory growth, not to
//     gate user-visible expiry.
//   - Clock seam (D-86): NewMemoryCache uses time.Now; the unexported
//     newMemoryCacheWithClock takes a custom nowFn for fake-clock tests
//     (TEST-06). The Client's WithCache(ttl) path uses time.Now literally
//     because cache construction happens before Client materializes its
//     own nowFunc; tests that need a fake clock route through
//     WithCacheBackend(newMemoryCacheWithClock(ttl, fc.Now)) instead.
//
// No init() and no package-level vars — keeps the CLIENT-10 AST audit in
// internal_test.go green without modification to its allowlist.

package openholidays

import (
	"context"
	"sync"
	"time"
)

// minSweeperInterval is the floor on the sweeper's tick interval (D-84).
// Short-TTL test caches sweep aggressively but never faster than this;
// long-TTL production caches sweep at ttl/4. The 30-second floor avoids
// burning CPU on caches whose TTL is sub-second (typical only in tests
// that exercise eviction; production TTLs are minutes or hours).
const minSweeperInterval = 30 * time.Second

// entry is the unexported value type stored under each key. value is the
// raw response bytes captured by cacheTransport (Plan 04 Task 2); expiresAt
// is the absolute instant at which Get must report a miss (D-81).
type entry struct {
	value     []byte
	expiresAt time.Time
}

// MemoryCache is the default in-memory TTL cache returned by NewMemoryCache
// (D-81). The backing storage is map[string]entry under [sync.RWMutex] —
// safe for concurrent use from any goroutine (CLIENT-07 / Pitfall CACHE-4).
//
// Instances are constructed via NewMemoryCache (or newMemoryCacheWithClock
// inside tests) and stopped via Close. The zero value is NOT usable —
// fields are populated by the constructor; copying a MemoryCache by value
// is not supported ([sync.RWMutex] and [sync.Once] trigger the standard go vet
// copy-lock warning).
//
// Lifecycle:
//
//   - construction: NewMemoryCache or newMemoryCacheWithClock allocates
//     the entries map and a sweeper context; NO goroutine is started yet.
//   - first Put: startOnce.Do(startSweeper) spawns exactly one goroutine
//     (D-84 lazy start).
//   - subsequent Put / Get: O(1) under the mutex.
//   - Close: cancels the sweeper context, briefly waits on the sweepDone
//     channel (1 ms cap), returns nil. Idempotent under closeOnce (D-85).
type MemoryCache struct {
	ttl         time.Duration
	nowFn       func() time.Time
	mu          sync.RWMutex
	entries     map[string]entry
	startOnce   sync.Once
	sweepCtx    context.Context
	sweepCancel context.CancelFunc
	sweepDone   chan struct{}
	closeOnce   sync.Once
}

// NewMemoryCache constructs a *MemoryCache backed by an in-memory map with
// the supplied TTL (D-79 / D-81). The cache uses [time.Now] as its clock; for
// fake-clock tests, use newMemoryCacheWithClock through a
// WithCacheBackend(...) wiring (D-86 documents the compromise — options
// run BEFORE Client construction, so WithCache(ttl) cannot pick up a
// Client-side nowFunc retroactively).
//
// The sweeper goroutine starts lazily on the first successful Put (D-84);
// a constructed-but-unused MemoryCache costs zero goroutines. Close is
// idempotent (D-85) and safe to call from any goroutine concurrently —
// the documented v1 cleanup idiom is defer client.Close().
//
// Caller contract on ttl (WR-02): callers MUST supply a positive ttl.
// A non-positive ttl (ttl <= 0) produces a constructed-but-useless
// cache — every Put stores an entry whose expiresAt is at or before
// now, so every subsequent Get returns (nil, false), AND the sweeper
// still spawns on first Put (wasting one goroutine for the cache's
// lifetime). The WithCache(ttl) option correctly rejects ttl <= 0
// (D-80); callers using NewMemoryCache directly via
// WithCacheBackend(NewMemoryCache(ttl)) must validate ttl themselves.
// NewMemoryCache does not panic on ttl <= 0 to preserve the library
// contract that constructors never error.
func NewMemoryCache(ttl time.Duration) *MemoryCache {
	return newMemoryCacheWithClock(ttl, time.Now)
}

// newMemoryCacheWithClock is the unexported constructor used by tests that
// need to drive the cache from a deterministic clock (TEST-06 / D-86). The
// supplied nowFn is invoked on every Put (to compute expiresAt), every Get
// (to check expiry), and every sweeper tick (to detect stale entries).
//
// Returning the same *MemoryCache type as NewMemoryCache lets tests assign
// the result to a Cache interface variable or pass it to WithCacheBackend
// without further type-juggling.
func newMemoryCacheWithClock(ttl time.Duration, nowFn func() time.Time) *MemoryCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &MemoryCache{
		ttl:         ttl,
		nowFn:       nowFn,
		entries:     make(map[string]entry),
		sweepCtx:    ctx,
		sweepCancel: cancel,
		sweepDone:   make(chan struct{}),
	}
}

// Get returns the cached bytes for key and true on a hit, or (nil, false)
// on a miss or expired entry (D-81 lazy-expiration-on-read). Safe for
// concurrent use.
//
// The RLock path is the hot path: a hit returns a defensive copy of the
// stored slice without acquiring the write lock. Stale entries are NOT
// deleted on Get — deletion is the sweeper's responsibility — but they
// are reported as a miss so callers observe the expected post-TTL
// behavior immediately.
//
// IN-05: the returned slice is a defensive copy of the internal byte
// buffer, NOT a reference. Callers may safely mutate or retain the
// returned slice without corrupting cache state. The copy cost for the
// typical 50-holiday JSON payload (<10 KiB) is negligible relative to
// the alternative footgun where a caller mutating the returned slice
// silently corrupts every subsequent cache read. This guarantee is
// part of the Cache interface contract (config.go).
func (m *MemoryCache) Get(key string) ([]byte, bool) {
	m.mu.RLock()
	e, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok || m.nowFn().After(e.expiresAt) {
		return nil, false
	}
	// Defensive copy — see Cache interface godoc / IN-05.
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, true
}

// Put stores value under key with an expiresAt of now + ttl. The first
// successful Put lazily starts the sweeper goroutine (D-84) — a
// constructed-but-unused MemoryCache spawns no goroutines until something
// is stored in it.
//
// Replacing an existing entry at key is supported and refreshes the TTL.
// Safe for concurrent use.
//
// IN-05: Put stores a defensive copy of value, NOT a reference. Callers
// may safely mutate or retain the supplied slice after Put returns
// without corrupting cache state. This guarantee mirrors the Get-side
// copy contract and is part of the Cache interface (config.go).
func (m *MemoryCache) Put(key string, value []byte) {
	// Defensive copy — see Cache interface godoc / IN-05.
	stored := make([]byte, len(value))
	copy(stored, value)
	m.mu.Lock()
	m.entries[key] = entry{value: stored, expiresAt: m.nowFn().Add(m.ttl)}
	m.mu.Unlock()
	m.startOnce.Do(m.startSweeper)
}

// startSweeper spawns the eviction goroutine using a tick interval of
// max(ttl/4, minSweeperInterval) (D-84). Called exactly once via
// startOnce.Do from the first Put.
func (m *MemoryCache) startSweeper() {
	interval := m.ttl / 4
	if interval < minSweeperInterval {
		interval = minSweeperInterval
	}
	go m.sweepLoop(interval)
}

// sweepLoop is the eviction goroutine body. It runs until sweepCtx is
// cancelled by Close, at which point defer close(sweepDone) signals Close
// that the loop has exited.
//
// On every tick the loop walks the entries map and deletes any whose
// expiresAt is in the past per nowFn. The walk holds the write lock — a
// brief stall on Get/Put around eviction time is the documented tradeoff
// for not maintaining a per-key timer (the alternative — N timers — does
// not scale and adds an own-goroutine-per-entry leak vector).
func (m *MemoryCache) sweepLoop(interval time.Duration) {
	defer close(m.sweepDone)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.sweepCtx.Done():
			return
		case <-t.C:
			m.evict()
		}
	}
}

// evict scans the entries map and deletes any expired entries. Called once
// per sweeper tick.
func (m *MemoryCache) evict() {
	now := m.nowFn()
	m.mu.Lock()
	for k, e := range m.entries {
		if now.After(e.expiresAt) {
			delete(m.entries, k)
		}
	}
	m.mu.Unlock()
}

// Close cancels the sweeper context and waits briefly for the sweeper
// goroutine to exit. Idempotent under closeOnce (D-85): concurrent Close
// calls from any number of goroutines all return nil without racing.
//
// The 1 ms timeout on the sweepDone wait is the deliberate cap: if the
// sweeper is in the middle of an evict() call holding the write lock,
// Close still returns within 1 ms rather than blocking indefinitely. The
// sweeper will exit shortly thereafter when it observes ctx.Done. This is
// best-effort cleanup per the Cache interface contract — Close's
// promise is "no further sweeper work after I return", not "the goroutine
// is definitely off the runtime queue".
//
// If the sweeper was never started (no Put was ever called), sweepDone
// stays open forever and the select takes the [time.After] branch — that's
// the intended path for an idle cache.
func (m *MemoryCache) Close() error {
	m.closeOnce.Do(func() {
		m.sweepCancel()
		select {
		case <-m.sweepDone:
		case <-time.After(time.Millisecond):
		}
	})
	return nil
}
