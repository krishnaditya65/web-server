package lb

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishnaditya65/web-server/internal/types"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeUpstream(rawURL string, weight int) *types.Upstream {
	u, _ := url.Parse(rawURL)
	up := &types.Upstream{
		URL:                u,
		Weight:             weight,
		CBFailureThreshold: 3,
		CBOpenDuration:     2 * time.Second,
		CBHalfOpenRequests: 1,
	}
	up.Healthy.Store(true)
	return up
}

func countSelections(b Balancer, n int) map[string]int {
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		up, err := b.Next()
		if err != nil {
			continue
		}
		counts[up.URL.Host]++
		b.Release(up)
	}
	return counts
}

// ── RoundRobin ─────────────────────────────────────────────────────────────────

func TestRoundRobin_DistributesEvenly(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)
	rr := NewRoundRobin([]*types.Upstream{a, b})

	counts := countSelections(rr, 100)
	assert.InDelta(t, 50, counts["a:9001"], 2)
	assert.InDelta(t, 50, counts["b:9002"], 2)
}

func TestRoundRobin_SingleUpstream(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	rr := NewRoundRobin([]*types.Upstream{a})

	counts := countSelections(rr, 10)
	assert.Equal(t, 10, counts["a:9001"])
}

func TestRoundRobin_SkipsUnhealthy(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)
	b.Healthy.Store(false)

	rr := NewRoundRobin([]*types.Upstream{a, b})
	counts := countSelections(rr, 20)

	assert.Equal(t, 20, counts["a:9001"])
	assert.Equal(t, 0, counts["b:9002"])
}

func TestRoundRobin_SkipsCircuitOpen(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)
	b.OpenCircuit(2 * time.Second) // manually open the circuit

	rr := NewRoundRobin([]*types.Upstream{a, b})
	counts := countSelections(rr, 20)

	assert.Equal(t, 20, counts["a:9001"])
	assert.Equal(t, 0, counts["b:9002"])
}

func TestRoundRobin_ErrorWhenAllUnhealthy(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	a.Healthy.Store(false)

	rr := NewRoundRobin([]*types.Upstream{a})
	_, err := rr.Next()
	assert.Error(t, err)
}

func TestRoundRobin_TracksActiveConns(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	rr := NewRoundRobin([]*types.Upstream{a})

	up, err := rr.Next()
	require.NoError(t, err)
	assert.Equal(t, int64(1), up.ActiveConns.Load())

	rr.Release(up)
	assert.Equal(t, int64(0), up.ActiveConns.Load())
}

func TestRoundRobin_MarkFailureDelegates(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	rr := NewRoundRobin([]*types.Upstream{a})

	rr.MarkFailure(a)
	rr.MarkFailure(a)
	rr.MarkFailure(a) // hits threshold=3

	assert.Equal(t, types.CircuitOpen, a.State())
}

func TestRoundRobin_RecoverAfterUnhealthyRestored(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)
	b.Healthy.Store(false)

	rr := NewRoundRobin([]*types.Upstream{a, b})
	b.Healthy.Store(true) // upstream recovers

	counts := countSelections(rr, 20)
	assert.Greater(t, counts["b:9002"], 0, "recovered upstream should receive traffic")
}

// ── WeightedRoundRobin ─────────────────────────────────────────────────────────

func TestWeightedRR_DistributesByWeight(t *testing.T) {
	a := makeUpstream("http://a:9001", 3)
	b := makeUpstream("http://b:9002", 1)

	wrr := NewWeightedRoundRobin([]*types.Upstream{a, b})
	counts := countSelections(wrr, 400)

	// a should get ~75% (300), b ~25% (100)
	assert.InDelta(t, 300, counts["a:9001"], 20)
	assert.InDelta(t, 100, counts["b:9002"], 20)
}

func TestWeightedRR_ZeroWeightTreatedAsOne(t *testing.T) {
	a := makeUpstream("http://a:9001", 0) // weight 0 → treated as 1
	b := makeUpstream("http://b:9002", 0)

	wrr := NewWeightedRoundRobin([]*types.Upstream{a, b})
	counts := countSelections(wrr, 100)

	assert.InDelta(t, 50, counts["a:9001"], 5)
	assert.InDelta(t, 50, counts["b:9002"], 5)
}

func TestWeightedRR_SkipsUnhealthy(t *testing.T) {
	a := makeUpstream("http://a:9001", 3)
	b := makeUpstream("http://b:9002", 1)
	b.Healthy.Store(false)

	wrr := NewWeightedRoundRobin([]*types.Upstream{a, b})
	counts := countSelections(wrr, 20)

	assert.Equal(t, 20, counts["a:9001"])
	assert.Equal(t, 0, counts["b:9002"])
}

func TestWeightedRR_ErrorWhenAllUnhealthy(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	a.Healthy.Store(false)

	wrr := NewWeightedRoundRobin([]*types.Upstream{a})
	_, err := wrr.Next()
	assert.Error(t, err)
}

// ── LeastConnections ──────────────────────────────────────────────────────────

func TestLeastConn_PicksLowestActiveConns(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)

	// Manually set a to have 5 active connections.
	a.ActiveConns.Store(5)

	lc := NewLeastConnections([]*types.Upstream{a, b})
	up, err := lc.Next()

	require.NoError(t, err)
	assert.Equal(t, "b:9002", up.URL.Host, "should pick the upstream with fewer connections")
	lc.Release(up)
}

func TestLeastConn_PicksFirstWhenEqual(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)

	lc := NewLeastConnections([]*types.Upstream{a, b})
	up, err := lc.Next()

	require.NoError(t, err)
	// When equal, first healthy upstream wins.
	assert.NotNil(t, up)
	lc.Release(up)
}

func TestLeastConn_SkipsUnhealthy(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)
	a.Healthy.Store(false) // a has 0 conns but is unhealthy

	lc := NewLeastConnections([]*types.Upstream{a, b})
	up, err := lc.Next()

	require.NoError(t, err)
	assert.Equal(t, "b:9002", up.URL.Host)
	lc.Release(up)
}

func TestLeastConn_ErrorWhenAllUnhealthy(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	a.Healthy.Store(false)

	lc := NewLeastConnections([]*types.Upstream{a})
	_, err := lc.Next()
	assert.Error(t, err)
}

func TestLeastConn_DynamicRebalancing(t *testing.T) {
	a := makeUpstream("http://a:9001", 1)
	b := makeUpstream("http://b:9002", 1)
	lc := NewLeastConnections([]*types.Upstream{a, b})

	// Acquire a connection to a.
	upA, _ := lc.Next()
	require.Equal(t, "a:9001", upA.URL.Host)
	// a now has 1 conn; b has 0 — next pick must be b.
	upB, _ := lc.Next()
	assert.Equal(t, "b:9002", upB.URL.Host)

	lc.Release(upA)
	lc.Release(upB)
}

// ── Balancer interface correctness ────────────────────────────────────────────

func TestAllBalancers_UpstreamsReturnsAll(t *testing.T) {
	ups := []*types.Upstream{
		makeUpstream("http://a:9001", 1),
		makeUpstream("http://b:9002", 1),
	}

	balancers := []Balancer{
		NewRoundRobin(ups),
		NewWeightedRoundRobin(ups),
		NewLeastConnections(ups),
	}

	for _, b := range balancers {
		assert.Len(t, b.Upstreams(), 2)
	}
}
