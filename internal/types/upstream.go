package types

import (
	"net/url"
	"sync/atomic"
	"time"
)

type CircuitState int32

const (
	CircuitClosed   CircuitState = 0
	CircuitOpen     CircuitState = 1
	CircuitHalfOpen CircuitState = 2
)

type Upstream struct {
	URL         *url.URL
	Healthy     atomic.Bool
	ActiveConns atomic.Int64
	Weight      int

	FailureCount     atomic.Int64
	CircuitOpenUntil atomic.Int64 // Unix seconds
	circuitState     atomic.Int32 // stores CircuitState

	halfOpenInFlight atomic.Int64

	// Set once at build time from route config.
	CBFailureThreshold int
	CBOpenDuration     time.Duration
	CBHalfOpenRequests int
}

func (u *Upstream) State() CircuitState {
	return CircuitState(u.circuitState.Load())
}

// CircuitOpen returns true when the upstream should not receive normal traffic.
func (u *Upstream) CircuitOpen() bool {
	switch u.State() {
	case CircuitOpen:
		return time.Now().Unix() < u.CircuitOpenUntil.Load()
	case CircuitHalfOpen:
		return u.halfOpenInFlight.Load() >= int64(u.CBHalfOpenRequests)
	default:
		return false
	}
}

// TryHalfOpen attempts to allow a probe request through when the circuit timer
// has expired. Returns true if the caller may proceed as a probe.
func (u *Upstream) TryHalfOpen() bool {
	state := u.State()

	if state == CircuitOpen {
		if time.Now().Unix() < u.CircuitOpenUntil.Load() {
			return false
		}
		// Timer expired — transition to half-open (only one goroutine wins the CAS).
		if !u.circuitState.CompareAndSwap(int32(CircuitOpen), int32(CircuitHalfOpen)) {
			// Another goroutine already transitioned; fall through to half-open check.
		} else {
			u.halfOpenInFlight.Store(0)
		}
		state = CircuitHalfOpen
	}

	if state == CircuitHalfOpen {
		maxProbes := int64(u.CBHalfOpenRequests)
		if maxProbes <= 0 {
			maxProbes = 1
		}
		for {
			cur := u.halfOpenInFlight.Load()
			if cur >= maxProbes {
				return false
			}
			if u.halfOpenInFlight.CompareAndSwap(cur, cur+1) {
				return true
			}
		}
	}

	return false
}

// RecordSuccess resets failure state. Call this on a successful upstream response.
func (u *Upstream) RecordSuccess() {
	state := u.State()

	if state == CircuitHalfOpen {
		u.halfOpenInFlight.Add(-1)
		u.circuitState.Store(int32(CircuitClosed))
		u.Healthy.Store(true)
	}

	u.FailureCount.Store(0)
}

// RecordFailure increments the failure counter and opens the circuit when the
// threshold is reached.
func (u *Upstream) RecordFailure() {
	threshold := int64(u.CBFailureThreshold)
	if threshold <= 0 {
		threshold = 3
	}

	dur := u.CBOpenDuration
	if dur <= 0 {
		dur = 30 * time.Second
	}

	n := u.FailureCount.Add(1)

	if u.State() == CircuitHalfOpen {
		u.halfOpenInFlight.Add(-1)
	}

	if n >= threshold {
		u.Healthy.Store(false)
		u.OpenCircuit(dur)
		u.FailureCount.Store(0)
	}
}

// OpenCircuit transitions the upstream to the open state for the given duration.
func (u *Upstream) OpenCircuit(d time.Duration) {
	u.CircuitOpenUntil.Store(time.Now().Add(d).Unix())
	u.circuitState.Store(int32(CircuitOpen))
}

