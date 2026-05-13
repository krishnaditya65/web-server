package proxy

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/plugin"
)

// buildTestGateway creates a Gateway for testing, pointing at real httptest servers.
func buildTestGateway(t *testing.T, routes []config.RouteConfig) (*Gateway, []*httptest.Server) {
	t.Helper()

	var servers []*httptest.Server

	for i := range routes {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("upstream"))
		}))
		t.Cleanup(srv.Close)
		servers = append(servers, srv)

		if len(routes[i].Upstreams) == 0 {
			routes[i].Upstreams = []config.UpstreamConfig{{URL: srv.URL, Weight: 1}}
		}
	}

	cfg := &config.Config{
		Proxy:  config.ProxyConfig{Algorithm: "round_robin", Routes: routes},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	reg := plugin.NewRegistry()
	gw := New(cfg, reg, zap.NewNop())

	return gw, servers
}

// ── Route matching ────────────────────────────────────────────────────────────

func TestGateway_LongestPrefixWins(t *testing.T) {
	var mu sync.Mutex
	var hitRoute string

	makeHandler := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hitRoute = name
			mu.Unlock()
			w.WriteHeader(200)
		})
	}

	srv1 := httptest.NewServer(makeHandler("users"))
	srv2 := httptest.NewServer(makeHandler("root"))
	t.Cleanup(srv1.Close)
	t.Cleanup(srv2.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{
				{Name: "users", PathPrefix: "/users", Upstreams: []config.UpstreamConfig{{URL: srv1.URL, Weight: 1}}},
				{Name: "root", PathPrefix: "/", Upstreams: []config.UpstreamConfig{{URL: srv2.URL, Weight: 1}}},
			},
		},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := New(cfg, plugin.NewRegistry(), zap.NewNop())
	testSrv := httptest.NewServer(gw)
	t.Cleanup(testSrv.Close)

	resp, err := http.Get(testSrv.URL + "/users/profile")
	require.NoError(t, err)
	resp.Body.Close()
	mu.Lock()
	assert.Equal(t, "users", hitRoute, "/users/profile must match the /users route")
	mu.Unlock()

	resp, err = http.Get(testSrv.URL + "/other")
	require.NoError(t, err)
	resp.Body.Close()
	mu.Lock()
	assert.Equal(t, "root", hitRoute, "/other must fall back to / route")
	mu.Unlock()
}

func TestGateway_Returns404WhenNoMatch(t *testing.T) {
	cfg := &config.Config{
		Proxy:  config.ProxyConfig{Routes: []config.RouteConfig{}},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	gw := New(cfg, plugin.NewRegistry(), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGateway_HostSpecificRouteWins(t *testing.T) {
	var mu sync.Mutex
	var hitRoute string

	makeUpstream := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hitRoute = name
			mu.Unlock()
			w.WriteHeader(200)
		}))
	}

	srvGeneric := makeUpstream("generic")
	srvSpecific := makeUpstream("specific")
	t.Cleanup(srvGeneric.Close)
	t.Cleanup(srvSpecific.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{
				{
					Name:       "generic",
					PathPrefix: "/",
					Upstreams:  []config.UpstreamConfig{{URL: srvGeneric.URL, Weight: 1}},
				},
				{
					Name:       "specific",
					Host:       "api.example.com",
					PathPrefix: "/",
					Upstreams:  []config.UpstreamConfig{{URL: srvSpecific.URL, Weight: 1}},
				},
			},
		},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := New(cfg, plugin.NewRegistry(), zap.NewNop())
	testSrv := httptest.NewServer(gw)
	t.Cleanup(testSrv.Close)

	req, _ := http.NewRequest(http.MethodGet, testSrv.URL+"/", nil)
	req.Host = "api.example.com"
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	mu.Lock()
	assert.Equal(t, "specific", hitRoute, "host-specific route should win")
	mu.Unlock()
}

// ── Reload ─────────────────────────────────────────────────────────────────────

func TestGateway_Reload(t *testing.T) {
	srvV1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v1"))
	}))
	srvV2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v2"))
	}))
	t.Cleanup(srvV1.Close)
	t.Cleanup(srvV2.Close)

	cfg1 := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes:    []config.RouteConfig{{Name: "r", PathPrefix: "/", Upstreams: []config.UpstreamConfig{{URL: srvV1.URL, Weight: 1}}}},
		},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg1)

	gw := New(cfg1, plugin.NewRegistry(), zap.NewNop())
	testSrv := httptest.NewServer(gw)
	t.Cleanup(testSrv.Close)

	// Before reload: should hit v1.
	resp, _ := http.Get(testSrv.URL + "/")
	body := make([]byte, 10)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	assert.Equal(t, "v1", string(body[:n]))

	// Reload with v2 upstream.
	cfg2 := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes:    []config.RouteConfig{{Name: "r", PathPrefix: "/", Upstreams: []config.UpstreamConfig{{URL: srvV2.URL, Weight: 1}}}},
		},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg2)
	require.NoError(t, gw.Reload(cfg2))

	// After reload: should hit v2.
	resp, _ = http.Get(testSrv.URL + "/")
	n, _ = resp.Body.Read(body)
	resp.Body.Close()
	assert.Equal(t, "v2", string(body[:n]))
}

// ── Routes snapshot ────────────────────────────────────────────────────────────

func TestGateway_RoutesSnapshot(t *testing.T) {
	srv := httptest.NewServer(okUpstreamHandler)
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Algorithm: "round_robin",
			Routes: []config.RouteConfig{
				{Name: "alpha", PathPrefix: "/alpha", Upstreams: []config.UpstreamConfig{{URL: srv.URL, Weight: 1}}},
				{Name: "beta", PathPrefix: "/beta", Upstreams: []config.UpstreamConfig{{URL: srv.URL, Weight: 1}}},
			},
		},
		Health: config.HealthConfig{Path: "/", IntervalSeconds: 60, TimeoutSeconds: 1},
	}
	config.ApplyDefaultsForTest(cfg)

	gw := New(cfg, plugin.NewRegistry(), zap.NewNop())
	routes := gw.Routes()

	assert.Len(t, routes, 2)
	names := []string{routes[0].Name, routes[1].Name}
	assert.ElementsMatch(t, []string{"alpha", "beta"}, names)
}

var okUpstreamHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
})
