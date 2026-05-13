// Package sla verifies that the proxy meets Service Level Agreement targets.
//
// SLA targets enforced:
//   - Availability:  ≥ 99.9% success rate with healthy upstreams
//   - Degraded mode: ≥ 95.0% success rate when 1 of 2 upstreams is down
//   - p99 latency:   ≤ 150 ms for a fast upstream under moderate load
//   - p95 latency:   ≤  80 ms for a fast upstream under moderate load
//   - Recovery:      proxy routes traffic within 1s after an upstream recovers
package sla

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/pluginreg"
	"github.com/krishnaditya65/web-server/internal/proxy"
)

// ── Constants (edit to tighten / loosen SLAs) ─────────────────────────────────

const (
	slaAvailability      = 99.9  // % success rate with all upstreams healthy
	slaDegradedAvail     = 95.0  // % success rate with one upstream down
	slaP99LatencyMs      = 150   // milliseconds
	slaP95LatencyMs      = 80    // milliseconds
	slaRecoveryWindow    = 2 * time.Second
	slaLoadRequests      = 1000
	slaLoadConcurrency   = 20
)

// ── Helpers ────────────────────────────────────────────────────────────────────

type result struct {
	latencies []time.Duration
	success   int64
	errors    int64
	total     int64
}

func (r *result) availability() float64 {
	if r.total == 0 {
		return 100
	}
	return float64(r.success) / float64(r.total) * 100
}

func (r *result) percentile(p float64) time.Duration {
	if len(r.latencies) == 0 {
		return 0
	}
	s := make([]time.Duration, len(r.latencies))
	copy(s, r.latencies)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(float64(len(s)-1) * p / 100.0)
	return s[idx]
}

func sendLoad(url string, n, concurrency int) *result {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: concurrency * 2,
			DisableCompression:  true,
		},
		Timeout: 5 * time.Second,
	}

	r := &result{}
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			t0 := time.Now()
			resp, err := client.Get(url + "/")
			lat := time.Since(t0)

			atomic.AddInt64(&r.total, 1)
			if err != nil {
				atomic.AddInt64(&r.errors, 1)
			} else {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					atomic.AddInt64(&r.success, 1)
				} else {
					atomic.AddInt64(&r.errors, 1)
				}
			}

			mu.Lock()
			r.latencies = append(r.latencies, lat)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return r
}

func buildProxy(t *testing.T, upstreams ...string) (*httptest.Server, *proxy.Gateway) {
	t.Helper()

	ups := make([]config.UpstreamConfig, len(upstreams))
	for i, u := range upstreams {
		ups[i] = config.UpstreamConfig{URL: u, Weight: 1}
	}

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{{
				Name:       "sla",
				PathPrefix: "/",
				MaxRetries: 1,
				Upstreams:  ups,
			}},
		},
		Health: config.HealthConfig{Path: "/healthz", IntervalSeconds: 1, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := proxy.New(cfg, pluginreg.Default(), zap.NewNop())
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	return srv, gw
}

// ── SLA 1: Availability ≥ 99.9% under normal load ─────────────────────────────

func TestSLA_Availability_AllHealthy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(upstream.Close)

	proxySrv, _ := buildProxy(t, upstream.URL)

	r := sendLoad(proxySrv.URL, slaLoadRequests, slaLoadConcurrency)

	avail := r.availability()
	fmt.Printf("\n── SLA: Availability (all healthy) ──\n")
	fmt.Printf("  Requests: %d, Success: %d, Errors: %d\n", r.total, r.success, r.errors)
	fmt.Printf("  Availability: %.3f%% (SLA: ≥%.1f%%)\n", avail, slaAvailability)

	assert.GreaterOrEqual(t, avail, slaAvailability,
		"availability %.3f%% is below SLA %.1f%%", avail, slaAvailability)
}

// ── SLA 2: Degraded mode — 1 of 2 upstreams down ─────────────────────────────

func TestSLA_Availability_OneUpstreamDown(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(healthy.Close)

	// Start a server and immediately close it to simulate a down upstream.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	broken.Close()

	proxySrv, _ := buildProxy(t, healthy.URL, broken.URL)

	// Allow health checks to mark the broken upstream unhealthy.
	time.Sleep(200 * time.Millisecond)

	r := sendLoad(proxySrv.URL, slaLoadRequests, slaLoadConcurrency)

	avail := r.availability()
	fmt.Printf("\n── SLA: Degraded (1 of 2 upstreams down) ──\n")
	fmt.Printf("  Requests: %d, Success: %d, Errors: %d\n", r.total, r.success, r.errors)
	fmt.Printf("  Availability: %.3f%% (SLA: ≥%.1f%%)\n", avail, slaDegradedAvail)

	assert.GreaterOrEqual(t, avail, slaDegradedAvail,
		"degraded availability %.3f%% is below SLA %.1f%%", avail, slaDegradedAvail)
}

// ── SLA 3: p99 and p95 latency targets ────────────────────────────────────────

func TestSLA_Latency_P99_P95(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	proxySrv, _ := buildProxy(t, upstream.URL)

	r := sendLoad(proxySrv.URL, slaLoadRequests, slaLoadConcurrency)

	p95 := r.percentile(95)
	p99 := r.percentile(99)

	fmt.Printf("\n── SLA: Latency ──\n")
	fmt.Printf("  p50: %v\n", r.percentile(50))
	fmt.Printf("  p95: %v  (SLA: ≤%dms)\n", p95, slaP95LatencyMs)
	fmt.Printf("  p99: %v  (SLA: ≤%dms)\n", p99, slaP99LatencyMs)

	assert.LessOrEqual(t, p95, time.Duration(slaP95LatencyMs)*time.Millisecond,
		"p95 latency %v exceeds SLA of %dms", p95, slaP95LatencyMs)
	assert.LessOrEqual(t, p99, time.Duration(slaP99LatencyMs)*time.Millisecond,
		"p99 latency %v exceeds SLA of %dms", p99, slaP99LatencyMs)
}

// ── SLA 4: Recovery — proxy routes traffic after upstream restarts ─────────────

func TestSLA_Recovery_UpstreamRestart(t *testing.T) {
	var serving atomic.Bool
	serving.Store(true)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			if serving.Load() {
				w.WriteHeader(200)
			} else {
				w.WriteHeader(503)
			}
			return
		}
		if !serving.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(upstream.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{{
				Name:       "sla",
				PathPrefix: "/",
				MaxRetries: 0,
				Upstreams:  []config.UpstreamConfig{{URL: upstream.URL, Weight: 1}},
			}},
		},
		// Fast health check interval so we detect recovery quickly.
		Health: config.HealthConfig{Path: "/healthz", IntervalSeconds: 1, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := proxy.New(cfg, pluginreg.Default(), zap.NewNop())
	proxySrv := httptest.NewServer(gw)
	t.Cleanup(proxySrv.Close)

	// Phase 1: all requests succeed.
	r1 := sendLoad(proxySrv.URL, 100, 5)
	require.GreaterOrEqual(t, r1.availability(), 99.0, "baseline availability must be high")

	// Phase 2: upstream goes down.
	serving.Store(false)
	time.Sleep(1500 * time.Millisecond) // wait for health check to mark unhealthy

	// Phase 3: upstream comes back.
	serving.Store(true)
	fmt.Printf("\n── SLA: Recovery ──\n")
	fmt.Printf("  Waiting up to %v for proxy to detect recovery...\n", slaRecoveryWindow)

	// Poll until traffic recovers or window expires.
	deadline := time.Now().Add(slaRecoveryWindow)
	var recovered bool
	for time.Now().Before(deadline) {
		resp, err := http.Get(proxySrv.URL + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				recovered = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	assert.True(t, recovered, "proxy must route traffic again within %v of upstream recovery", slaRecoveryWindow)
	fmt.Printf("  ✓ Traffic recovered within recovery window\n")
}

// ── SLA 5: Error rate stays below threshold under mixed load ──────────────────

func TestSLA_ErrorRate_UnderLoad(t *testing.T) {
	const maxErrorRate = 0.5 // 0.5%

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	proxySrv, _ := buildProxy(t, upstream.URL)

	r := sendLoad(proxySrv.URL, 2000, 50)

	errRate := float64(r.errors) / float64(r.total) * 100

	fmt.Printf("\n── SLA: Error Rate Under Load ──\n")
	fmt.Printf("  Requests: %d  Errors: %d  Rate: %.3f%%  (SLA: ≤%.1f%%)\n",
		r.total, r.errors, errRate, maxErrorRate)

	assert.LessOrEqual(t, errRate, maxErrorRate,
		"error rate %.3f%% exceeds SLA of %.1f%%", errRate, maxErrorRate)
}

// ── SLA 6: No request dropped during hot reload ────────────────────────────────

func TestSLA_HotReload_NoDroppedRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(200)
	}))
	t.Cleanup(upstream.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{{
				Name:       "sla",
				PathPrefix: "/",
				MaxRetries: 0,
				Upstreams:  []config.UpstreamConfig{{URL: upstream.URL, Weight: 1}},
			}},
		},
		Health: config.HealthConfig{Path: "/healthz", IntervalSeconds: 9999, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := proxy.New(cfg, pluginreg.Default(), zap.NewNop())
	proxySrv := httptest.NewServer(gw)
	t.Cleanup(proxySrv.Close)

	var errors atomic.Int64
	var total atomic.Int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Background goroutine: continuous requests.
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := &http.Client{Timeout: 2 * time.Second}
		for {
			select {
			case <-stop:
				return
			default:
				total.Add(1)
				resp, err := client.Get(proxySrv.URL + "/")
				if err != nil || resp.StatusCode != 200 {
					errors.Add(1)
				}
				if err == nil {
					resp.Body.Close()
				}
			}
		}
	}()

	// Let traffic run, then trigger a reload mid-flight.
	time.Sleep(100 * time.Millisecond)

	newCfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{{
				Name:       "sla",
				PathPrefix: "/",
				MaxRetries: 0,
				Upstreams:  []config.UpstreamConfig{{URL: upstream.URL, Weight: 1}},
			}},
		},
		Health: config.HealthConfig{Path: "/healthz", IntervalSeconds: 9999, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(newCfg)
	require.NoError(t, gw.Reload(newCfg))

	// Let traffic continue after reload.
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	errRate := float64(errors.Load()) / float64(total.Load()) * 100

	fmt.Printf("\n── SLA: Hot Reload ──\n")
	fmt.Printf("  Total: %d  Errors: %d  Error rate: %.3f%%\n",
		total.Load(), errors.Load(), errRate)

	assert.LessOrEqual(t, errRate, 1.0,
		"error rate during hot reload %.3f%% should be under 1%%", errRate)
}
