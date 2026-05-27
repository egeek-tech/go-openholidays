// Package openholidays — tests for Client construction and lifecycle.
//
// One TestXxx per exported production function (Gold Rule 3): TestNewClient
// (defaults + option composition) and TestClient_Close (idempotent + race-safe
// from any goroutine).
//
// NOTE: TestClient_ConcurrentAccess (CLIENT-07) and TestClient_ContextCancel
// (CLIENT-09) require an end-to-end HTTP call through the chain. Plan 02-02
// ships only the construction-time contract; those two tests land in Plan
// 02-03 alongside the Countries endpoint. The Close-under-100-goroutines
// subtest below is the Phase 2 Plan 02 mechanical race-safety check for the
// `closed` atomic flag specifically.

package openholidays

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClient covers CLIENT-01: defaults applied when no Option supplied,
// option composition (later Options override earlier ones for the same
// field), and the combined-options happy path.
func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("defaults applied when no Option supplied", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		assert.Equal(t, "https://openholidaysapi.org", c.baseURL,
			"default baseURL must match D-36 / PROJECT.md")
		assert.Equal(t, "go-openholidays/"+Version, c.userAgent,
			"default userAgent must be go-openholidays/<Version>")
		assert.Equal(t, 15*time.Second, c.timeout,
			"default timeout must be 15s per CLIENT-06 / D-28")
		require.NotNil(t, c.logger, "default logger must be non-nil (slog.Default())")
		require.NotNil(t, c.http, "default http client must be non-nil")
	})

	t.Run("Options compose left-to-right (later wins)", func(t *testing.T) {
		t.Parallel()
		c := NewClient(
			WithBaseURL("https://first.test"),
			WithBaseURL("https://second.test"),
		)
		require.NotNil(t, c)
		assert.Equal(t, "https://second.test", c.baseURL,
			"later Option must override earlier Option for the same field")
	})

	t.Run("WithHTTPClient + WithTimeout combine", func(t *testing.T) {
		t.Parallel()
		custom := &http.Client{}
		c := NewClient(
			WithHTTPClient(custom),
			WithTimeout(5*time.Second),
		)
		require.NotNil(t, c)
		require.NotNil(t, c.http)
		assert.Equal(t, 5*time.Second, c.timeout,
			"WithTimeout must be applied even when WithHTTPClient also supplied")
	})
}

// TestClient_Close covers CLIENT-08 / D-40: idempotent (every call returns
// nil, subsequent calls still return nil), and race-safe from any goroutine
// (100 parallel goroutines under -race all return nil and the final flag is
// true).
func TestClient_Close(t *testing.T) {
	t.Parallel()

	t.Run("first call returns nil and flips closed flag", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		require.False(t, c.closed.Load(),
			"closed should be false immediately after NewClient")
		err := c.Close()
		assert.NoError(t, err, "Close must return nil")
		assert.True(t, c.closed.Load(),
			"closed flag must be true after the first Close call")
	})

	t.Run("subsequent calls also return nil (idempotent)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		for i := 0; i < 5; i++ {
			assert.NoError(t, c.Close(),
				"Close call %d must return nil (idempotent per CLIENT-08)", i+1)
		}
		assert.True(t, c.closed.Load(),
			"closed must remain true after multiple Close calls")
	})

	t.Run("concurrent close is race-safe (100 goroutines)", func(t *testing.T) {
		t.Parallel()
		c := NewClient()
		require.NotNil(t, c)
		var wg sync.WaitGroup
		const N = 100
		for i := 0; i < N; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, c.Close(),
					"Close from a goroutine must return nil (CLIENT-08 / D-40)")
			}()
		}
		wg.Wait()
		assert.True(t, c.closed.Load(),
			"closed must be true after all goroutines have called Close")
	})
}
