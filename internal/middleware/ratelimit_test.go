package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/krishnaditya65/web-server/internal/config"
)

// okHandler is a simple 200 OK handler used as the inner handler in tests.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func doRequest(handler http.Handler, remoteAddr string) int {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

// ── Per-IP independence ────────────────────────────────────────────────────────

func TestRateLimit_DifferentIPsAreIndependent(t *testing.T) {
	mw := NewIPRateLimit(5, 5)
	handler := mw(okHandler)

	// Exhaust the limit for IP 1.
	for i := 0; i < 5; i++ {
		assert.Equal(t, 200, doRequest(handler, "1.1.1.1:1234"))
	}
	// IP 1 should now be limited.
	assert.Equal(t, 429, doRequest(handler, "1.1.1.1:1234"), "IP 1 should be rate-limited")
	// IP 2 should still be fine.
	assert.Equal(t, 200, doRequest(handler, "2.2.2.2:1234"), "IP 2 must not be affected")
}

// ── Burst behaviour ────────────────────────────────────────────────────────────

func TestRateLimit_AllowsUpToBurst(t *testing.T) {
	mw := NewIPRateLimit(1, 5) // 1 r/s, burst=5
	handler := mw(okHandler)

	// First 5 requests should go through (burst).
	for i := 0; i < 5; i++ {
		assert.Equal(t, 200, doRequest(handler, "3.3.3.3:1234"), "request %d should succeed", i+1)
	}
	// 6th request over burst should be limited.
	assert.Equal(t, 429, doRequest(handler, "3.3.3.3:1234"))
}

// ── Global config integration ──────────────────────────────────────────────────

func TestRateLimit_GlobalConfig(t *testing.T) {
	cfg := &config.Config{
		Rate: config.RateConfig{RequestsPerSecond: 2, Burst: 2},
	}
	mw := RateLimit(cfg)
	handler := mw(okHandler)

	assert.Equal(t, 200, doRequest(handler, "4.4.4.4:1234"))
	assert.Equal(t, 200, doRequest(handler, "4.4.4.4:1234"))
	assert.Equal(t, 429, doRequest(handler, "4.4.4.4:1234"))
}

// ── X-Forwarded-For handling ───────────────────────────────────────────────────

func TestRateLimit_UsesXForwardedFor(t *testing.T) {
	mw := NewIPRateLimit(1, 2)
	handler := mw(okHandler)

	send := func(xff string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, 200, send("5.5.5.5"))
	assert.Equal(t, 200, send("5.5.5.5"))
	assert.Equal(t, 429, send("5.5.5.5"), "XFF IP should be throttled")
	// A different XFF IP should still be fine.
	assert.Equal(t, 200, send("6.6.6.6"))
}

// ── clientIP helper ────────────────────────────────────────────────────────────

func TestClientIP_ParsesRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:54321"
	assert.Equal(t, "192.168.1.1", clientIP(req))
}

func TestClientIP_PrefersXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	assert.Equal(t, "203.0.113.5", clientIP(req))
}

func TestClientIP_FallsBackWhenXFFEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "172.16.0.1:8080"
	assert.Equal(t, "172.16.0.1", clientIP(req))
}
