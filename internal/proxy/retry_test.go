package proxy

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ── isReplaySafeMethod ────────────────────────────────────────────────────────

func TestIsReplaySafeMethod(t *testing.T) {
	safe := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	unsafe := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

	for _, m := range safe {
		assert.True(t, isReplaySafeMethod(m), "%s should be replay-safe", m)
	}

	for _, m := range unsafe {
		assert.False(t, isReplaySafeMethod(m), "%s must not be replay-safe", m)
	}
}

// ── shouldRetry ───────────────────────────────────────────────────────────────

func TestShouldRetry_RetryableStatusCodes(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)

	for _, code := range []int{502, 503, 504} {
		resp := &http.Response{StatusCode: code}
		assert.True(t, shouldRetry(req, resp, nil), "status %d should trigger retry", code)
	}
}

func TestShouldRetry_NonRetryableStatusCodes(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)

	for _, code := range []int{200, 201, 400, 401, 403, 404, 500} {
		resp := &http.Response{StatusCode: code}
		assert.False(t, shouldRetry(req, resp, nil), "status %d must not trigger retry", code)
	}
}

func TestShouldRetry_NoRetryOnUnsafeMethod(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, _ := http.NewRequest(method, "/", nil)
		resp := &http.Response{StatusCode: 503}
		assert.False(t, shouldRetry(req, resp, nil), "%s must not be retried", method)
	}
}

func TestShouldRetry_RetryOnTransportError(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	assert.True(t, shouldRetry(req, nil, syscall.ECONNRESET))
	assert.True(t, shouldRetry(req, nil, syscall.EPIPE))
}

func TestShouldRetry_NoRetryOnTransportErrorForPost(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	assert.False(t, shouldRetry(req, nil, syscall.ECONNRESET))
}

// ── RetryDelay ────────────────────────────────────────────────────────────────

func TestRetryDelay_AlwaysNonNegative(t *testing.T) {
	for i := 0; i < 10; i++ {
		d := RetryDelay(i)
		assert.GreaterOrEqual(t, d, time.Duration(0), "delay must be non-negative")
	}
}

func TestRetryDelay_CappedAt1Second(t *testing.T) {
	// Run many times to catch high random values.
	for i := 0; i < 100; i++ {
		d := RetryDelay(10) // large attempt number
		assert.LessOrEqual(t, d, time.Second, "delay must not exceed 1 second")
	}
}

func TestRetryDelay_Attempt0SmallThanAttempt5(t *testing.T) {
	// On average, later attempts should have higher delays.
	var sum0, sum5 time.Duration
	const samples = 1000
	for i := 0; i < samples; i++ {
		sum0 += RetryDelay(0)
		sum5 += RetryDelay(5)
	}
	avg0 := sum0 / samples
	avg5 := sum5 / samples
	assert.Less(t, avg0, avg5, "average delay for attempt 0 must be less than attempt 5")
}
