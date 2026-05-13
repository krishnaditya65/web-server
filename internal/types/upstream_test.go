package types

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestUpstream creates an Upstream with sensible test defaults.
// newTestUpstream creates an Upstream with sensible test defaults.
// CBOpenDuration is 2s because CircuitOpenUntil is stored as Unix seconds
// (1-second precision), so sub-second durations round to zero.
func newTestUpstream() *Upstream {
	u := &Upstream{
		CBFailureThreshold: 3,
		CBOpenDuration:     2 * time.Second,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)
	return u
}

// ── Initial state ──────────────────────────────────────────────────────────────

func TestUpstream_InitialStateClosed(t *testing.T) {
	u := newTestUpstream()
	assert.Equal(t, CircuitClosed, u.State())
	assert.False(t, u.CircuitOpen())
}

func TestUpstream_InitialHealthy(t *testing.T) {
	u := newTestUpstream()
	assert.True(t, u.Healthy.Load())
}

// ── RecordFailure / circuit opening ────────────────────────────────────────────

func TestUpstream_FailureBelowThreshold(t *testing.T) {
	u := newTestUpstream()

	u.RecordFailure()
	u.RecordFailure()

	assert.Equal(t, CircuitClosed, u.State(), "circuit must stay closed below threshold")
	assert.False(t, u.CircuitOpen())
	assert.True(t, u.Healthy.Load())
}

func TestUpstream_CircuitOpensAtThreshold(t *testing.T) {
	u := newTestUpstream()

	u.RecordFailure()
	u.RecordFailure()
	u.RecordFailure() // hits threshold=3

	assert.Equal(t, CircuitOpen, u.State())
	assert.True(t, u.CircuitOpen())
	assert.False(t, u.Healthy.Load())
}

func TestUpstream_FailureCountResetAfterOpen(t *testing.T) {
	u := newTestUpstream()

	u.RecordFailure()
	u.RecordFailure()
	u.RecordFailure()

	assert.Equal(t, int64(0), u.FailureCount.Load(), "counter must reset after circuit opens")
}

// ── TryHalfOpen ────────────────────────────────────────────────────────────────

func TestUpstream_TryHalfOpenRejectedWhileTimerActive(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     10 * time.Second, // far future
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)
	u.RecordFailure()

	require.Equal(t, CircuitOpen, u.State())
	assert.False(t, u.TryHalfOpen(), "probe must be blocked while timer is active")
}

func TestUpstream_TryHalfOpenAllowedAfterTimer(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)
	u.RecordFailure()

	time.Sleep(20 * time.Millisecond)

	assert.True(t, u.TryHalfOpen(), "probe must be allowed after timer expires")
	assert.Equal(t, CircuitHalfOpen, u.State())
}

func TestUpstream_TryHalfOpenRespectsMaxProbes(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 2,
	}
	u.Healthy.Store(true)
	u.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	assert.True(t, u.TryHalfOpen())  // probe 1 — allowed
	assert.True(t, u.TryHalfOpen())  // probe 2 — allowed
	assert.False(t, u.TryHalfOpen()) // probe 3 — denied (max=2)
}

// ── RecordSuccess ──────────────────────────────────────────────────────────────

func TestUpstream_RecordSuccessClosesFromHalfOpen(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)
	u.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	ok := u.TryHalfOpen()
	require.True(t, ok)

	u.RecordSuccess()

	assert.Equal(t, CircuitClosed, u.State(), "circuit must close after successful probe")
	assert.True(t, u.Healthy.Load())
	assert.Equal(t, int64(0), u.FailureCount.Load())
}

func TestUpstream_RecordSuccessInClosedResetsCounter(t *testing.T) {
	u := newTestUpstream()

	u.RecordFailure()
	u.RecordFailure()
	u.RecordSuccess()

	assert.Equal(t, int64(0), u.FailureCount.Load())
	assert.Equal(t, CircuitClosed, u.State())
}

// ── RecordFailure from HalfOpen ────────────────────────────────────────────────

func TestUpstream_RecordFailureFromHalfOpenReopens(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)
	u.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	u.TryHalfOpen()
	u.RecordFailure() // probe fails

	assert.Equal(t, CircuitOpen, u.State(), "failure during probe must reopen circuit")
	assert.False(t, u.Healthy.Load())
}

// ── Custom threshold / duration ────────────────────────────────────────────────

func TestUpstream_CustomThreshold(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 5,
		CBOpenDuration:     50 * time.Millisecond,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)

	for i := 0; i < 4; i++ {
		u.RecordFailure()
	}
	assert.Equal(t, CircuitClosed, u.State(), "must not open before threshold")

	u.RecordFailure() // 5th — at threshold
	assert.Equal(t, CircuitOpen, u.State())
}

func TestUpstream_OpenCircuitDuration(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     2 * time.Second,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)
	u.RecordFailure()

	assert.True(t, u.CircuitOpen(), "should be open immediately after failure threshold")

	// Manually expire the circuit by backdating the timer.
	u.CircuitOpenUntil.Store(time.Now().Add(-1 * time.Second).Unix())
	assert.False(t, u.CircuitOpen(), "should not be open once timer has expired")
}

// ── Concurrency: no data races ─────────────────────────────────────────────────

func TestUpstream_ConcurrentRecordFailureNoPanic(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1000, // high threshold so we don't flip state mid-test
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 1,
	}
	u.Healthy.Store(true)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u.RecordFailure()
		}()
	}
	wg.Wait()

	assert.GreaterOrEqual(t, u.FailureCount.Load(), int64(0))
}

func TestUpstream_ConcurrentTryHalfOpenOnlyOneWins(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 1,
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 1, // exactly 1 probe allowed
	}
	u.Healthy.Store(true)
	u.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	var wins atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if u.TryHalfOpen() {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), wins.Load(), "exactly one goroutine should win the probe slot")
}

func TestUpstream_ConcurrentMixedOperationsNoPanic(t *testing.T) {
	u := &Upstream{
		CBFailureThreshold: 5,
		CBOpenDuration:     10 * time.Millisecond,
		CBHalfOpenRequests: 2,
	}
	u.Healthy.Store(true)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			switch n % 3 {
			case 0:
				u.RecordFailure()
			case 1:
				u.RecordSuccess()
			case 2:
				u.TryHalfOpen()
			}
		}(i)
	}
	wg.Wait()
	// Test passes as long as it doesn't panic or deadlock (run with -race flag).
}
