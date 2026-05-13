// Package performance measures proxy throughput and latency at different concurrency levels.
// Run with: go test ./tests/performance/... -v -timeout 120s
// For Go benchmarks: go test ./tests/performance/... -bench=. -benchtime=5s
package performance

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

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/pluginreg"
	"github.com/krishnaditya65/web-server/internal/proxy"
)

// ── Test setup ────────────────────────────────────────────────────────────────

func newProxySrv(t testing.TB, upstreamHandler http.Handler) *httptest.Server {
	t.Helper()

	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{{
				Name:       "bench",
				PathPrefix: "/",
				MaxRetries: 0,
				Upstreams:  []config.UpstreamConfig{{URL: upstream.URL, Weight: 1}},
			}},
		},
		Health: config.HealthConfig{Path: "/healthz", IntervalSeconds: 9999, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := proxy.New(cfg, pluginreg.Default(), zap.NewNop())
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	return srv
}

// fastUpstream returns 200 OK immediately with a small JSON body.
var fastUpstream = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ok"}`))
})

// slowUpstream simulates a 5 ms upstream (e.g., a database query).
var slowUpstream = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	time.Sleep(5 * time.Millisecond)
	w.WriteHeader(200)
	w.Write([]byte(`{"status":"ok"}`))
})

// ── Stats ─────────────────────────────────────────────────────────────────────

type loadResult struct {
	Total     int
	Success   int
	Errors    int
	Duration  time.Duration
	Latencies []time.Duration
}

func (r *loadResult) RPS() float64 {
	return float64(r.Total) / r.Duration.Seconds()
}

func (r *loadResult) ErrorRate() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Errors) / float64(r.Total) * 100
}

func (r *loadResult) Percentile(p float64) time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(r.Latencies))
	copy(sorted, r.Latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

func (r *loadResult) Mean() time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, l := range r.Latencies {
		total += l
	}
	return total / time.Duration(len(r.Latencies))
}

func (r *loadResult) Print(label string) {
	fmt.Printf("\n── %s ──\n", label)
	fmt.Printf("  Total:      %d requests\n", r.Total)
	fmt.Printf("  Success:    %d (%.1f%%)\n", r.Success, float64(r.Success)/float64(r.Total)*100)
	fmt.Printf("  Errors:     %d (%.2f%%)\n", r.Errors, r.ErrorRate())
	fmt.Printf("  Duration:   %v\n", r.Duration.Round(time.Millisecond))
	fmt.Printf("  Throughput: %.0f req/s\n", r.RPS())
	fmt.Printf("  Latency  p50: %v\n", r.Percentile(50))
	fmt.Printf("  Latency  p95: %v\n", r.Percentile(95))
	fmt.Printf("  Latency  p99: %v\n", r.Percentile(99))
	fmt.Printf("  Latency p999: %v\n", r.Percentile(99.9))
	fmt.Printf("  Latency mean: %v\n", r.Mean())
}

// runLoad sends `requests` requests across `concurrency` goroutines and collects stats.
func runLoad(proxySrvURL string, requests, concurrency int) loadResult {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency * 2,
			DisableCompression:  true,
		},
		Timeout: 5 * time.Second,
	}

	var (
		mu        sync.Mutex
		latencies = make([]time.Duration, 0, requests)
		success   atomic.Int64
		errCount  atomic.Int64
	)

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < requests; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			t0 := time.Now()
			resp, err := client.Get(proxySrvURL + "/")
			lat := time.Since(t0)

			if err != nil {
				errCount.Add(1)
			} else {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == 200 {
					success.Add(1)
				} else {
					errCount.Add(1)
				}
			}

			mu.Lock()
			latencies = append(latencies, lat)
			mu.Unlock()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return loadResult{
		Total:     requests,
		Success:   int(success.Load()),
		Errors:    int(errCount.Load()),
		Duration:  elapsed,
		Latencies: latencies,
	}
}

// ── Go Benchmarks (run with -bench=.) ────────────────────────────────────────

func BenchmarkProxy_FastUpstream(b *testing.B) {
	srv := newProxySrv(b, fastUpstream)

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 32,
			DisableCompression:  true,
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(srv.URL + "/")
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	})
}

func BenchmarkProxy_SlowUpstream(b *testing.B) {
	srv := newProxySrv(b, slowUpstream)

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 64,
			DisableCompression:  true,
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(srv.URL + "/")
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	})
}

// ── Load tests (run with -v -run TestLoad) ────────────────────────────────────

func TestLoad_Concurrency1(t *testing.T) {
	srv := newProxySrv(t, fastUpstream)
	r := runLoad(srv.URL, 500, 1)
	r.Print("concurrency=1 fast-upstream")

	assert.LessOrEqual(t, r.ErrorRate(), 1.0, "error rate must stay below 1%")
	assert.LessOrEqual(t, r.Percentile(99), 100*time.Millisecond, "p99 must be under 100ms")
}

func TestLoad_Concurrency10(t *testing.T) {
	srv := newProxySrv(t, fastUpstream)
	r := runLoad(srv.URL, 1000, 10)
	r.Print("concurrency=10 fast-upstream")

	assert.LessOrEqual(t, r.ErrorRate(), 1.0, "error rate must stay below 1%")
	assert.LessOrEqual(t, r.Percentile(99), 200*time.Millisecond, "p99 must be under 200ms")
}

func TestLoad_Concurrency50(t *testing.T) {
	srv := newProxySrv(t, fastUpstream)
	r := runLoad(srv.URL, 2000, 50)
	r.Print("concurrency=50 fast-upstream")

	assert.LessOrEqual(t, r.ErrorRate(), 1.0, "error rate must stay below 1%")
	assert.LessOrEqual(t, r.Percentile(95), 500*time.Millisecond, "p95 must be under 500ms")
}

func TestLoad_Concurrency100(t *testing.T) {
	srv := newProxySrv(t, fastUpstream)
	r := runLoad(srv.URL, 3000, 100)
	r.Print("concurrency=100 fast-upstream")

	assert.LessOrEqual(t, r.ErrorRate(), 2.0, "error rate must stay below 2%")
}

func TestLoad_SlowUpstream_Concurrency20(t *testing.T) {
	srv := newProxySrv(t, slowUpstream)
	r := runLoad(srv.URL, 500, 20)
	r.Print("concurrency=20 slow-upstream (5ms)")

	assert.LessOrEqual(t, r.ErrorRate(), 1.0, "error rate must stay below 1%")
	// With 5ms upstream and 20 concurrent, mean latency should be close to 5ms.
	assert.LessOrEqual(t, r.Mean(), 50*time.Millisecond, "mean latency should be near upstream delay")
}

// ── Scalability: latency should not grow linearly with concurrency ─────────────

func TestLoad_ScalabilityProfile(t *testing.T) {
	srv := newProxySrv(t, fastUpstream)

	levels := []int{1, 5, 10, 25, 50}
	fmt.Println("\n── Scalability Profile (fast-upstream) ──")
	fmt.Printf("%-15s %-12s %-12s %-12s %-12s\n", "Concurrency", "RPS", "p50", "p95", "p99")

	var prevP99 time.Duration
	for _, c := range levels {
		r := runLoad(srv.URL, c*50, c)
		fmt.Printf("%-15d %-12.0f %-12v %-12v %-12v\n",
			c, r.RPS(), r.Percentile(50), r.Percentile(95), r.Percentile(99))

		if prevP99 > 0 {
			// p99 should not more than 10× the previous level's p99 (sanity check).
			assert.LessOrEqual(t, r.Percentile(99), prevP99*10,
				"p99 at concurrency=%d should not explode vs concurrency=%d", c, c/5)
		}
		prevP99 = r.Percentile(99)
	}
}

// ── Throughput: proxy overhead vs direct ──────────────────────────────────────

func TestLoad_ProxyOverhead(t *testing.T) {
	upstream := httptest.NewServer(fastUpstream)
	t.Cleanup(upstream.Close)

	proxySrv := newProxySrv(t, fastUpstream)

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 20,
			DisableCompression:  true,
		},
	}
	const requests = 500
	const concurrency = 10

	// Measure direct upstream.
	directResult := runLoad(upstream.URL, requests, concurrency)

	// Measure through proxy.
	proxyResult := runLoad(proxySrv.URL, requests, concurrency)

	directResult.Print("direct upstream")
	proxyResult.Print("through proxy")

	overhead := proxyResult.Mean() - directResult.Mean()
	_ = client
	fmt.Printf("\n  Proxy overhead (mean latency): %v\n", overhead)

	// Proxy overhead should be less than 50ms on average.
	assert.LessOrEqual(t, overhead, 50*time.Millisecond,
		"proxy mean latency overhead should be under 50ms")
}

// ── Two upstreams with weighted round-robin ───────────────────────────────────

func TestLoad_WeightedRoundRobin(t *testing.T) {
	var hits [2]atomic.Int64

	upstream0 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[0].Add(1)
		w.WriteHeader(200)
	}))
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[1].Add(1)
		w.WriteHeader(200)
	}))
	t.Cleanup(upstream0.Close)
	t.Cleanup(upstream1.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "weighted_rr",
			Routes: []config.RouteConfig{{
				Name:       "bench",
				PathPrefix: "/",
				MaxRetries: 0,
				Upstreams: []config.UpstreamConfig{
					{URL: upstream0.URL, Weight: 3},
					{URL: upstream1.URL, Weight: 1},
				},
			}},
		},
		Health: config.HealthConfig{Path: "/healthz", IntervalSeconds: 9999, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := proxy.New(cfg, pluginreg.Default(), zap.NewNop())
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)

	r := runLoad(srv.URL, 400, 10)
	r.Print("weighted_rr 3:1")

	assert.LessOrEqual(t, r.ErrorRate(), 1.0)

	// upstream0 should get ~75%, upstream1 ~25% (±10%).
	total := float64(hits[0].Load() + hits[1].Load())
	pct0 := float64(hits[0].Load()) / total * 100
	fmt.Printf("  upstream0: %.1f%%, upstream1: %.1f%%\n", pct0, 100-pct0)
	assert.InDelta(t, 75.0, pct0, 10.0, "upstream0 should receive ~75%% of traffic")
}
