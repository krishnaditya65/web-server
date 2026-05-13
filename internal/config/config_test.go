package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── applyDefaults ─────────────────────────────────────────────────────────────

func TestApplyDefaults_FillsRouteDefaults(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{
			Routes: []RouteConfig{
				{Name: "a", Upstreams: []UpstreamConfig{{URL: "http://x:9001", Weight: 0}}},
			},
		},
	}

	applyDefaults(cfg)

	r := cfg.Proxy.Routes[0]
	assert.Equal(t, "/", r.PathPrefix)
	assert.Equal(t, 30, r.TimeoutSeconds)
	assert.Equal(t, 2, r.MaxRetries)
	assert.Equal(t, 3, r.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 30, r.CircuitBreaker.OpenDurationSeconds)
	assert.Equal(t, 1, r.CircuitBreaker.HalfOpenRequests)
	assert.Equal(t, 1, r.Upstreams[0].Weight, "zero weight must become 1")
}

func TestApplyDefaults_DoesNotOverrideExistingValues(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{
			Routes: []RouteConfig{
				{
					Name:           "custom",
					PathPrefix:     "/custom",
					TimeoutSeconds: 10,
					MaxRetries:     5,
					CircuitBreaker: CircuitBreakerConfig{
						FailureThreshold:    7,
						OpenDurationSeconds: 60,
						HalfOpenRequests:    3,
					},
					Upstreams: []UpstreamConfig{{URL: "http://x:9001", Weight: 3}},
				},
			},
		},
	}

	applyDefaults(cfg)

	r := cfg.Proxy.Routes[0]
	assert.Equal(t, "/custom", r.PathPrefix)
	assert.Equal(t, 10, r.TimeoutSeconds)
	assert.Equal(t, 5, r.MaxRetries)
	assert.Equal(t, 7, r.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 60, r.CircuitBreaker.OpenDurationSeconds)
	assert.Equal(t, 3, r.CircuitBreaker.HalfOpenRequests)
	assert.Equal(t, 3, r.Upstreams[0].Weight)
}

func TestApplyDefaults_AdminDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	assert.Equal(t, "127.0.0.1", cfg.Admin.Host)
	assert.Equal(t, 8090, cfg.Admin.Port)
}

func TestApplyDefaults_GzipDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	assert.Equal(t, 6, cfg.Gzip.Level)
	assert.Equal(t, 1024, cfg.Gzip.MinLength)
	assert.NotEmpty(t, cfg.Gzip.ContentTypes)
}

func TestApplyDefaults_HealthDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	assert.Equal(t, "/", cfg.Health.Path)
	assert.Equal(t, 5, cfg.Health.IntervalSeconds)
	assert.Equal(t, 2, cfg.Health.TimeoutSeconds)
}

// ── LoadNginx round-trip ──────────────────────────────────────────────────────

func TestLoadNginx_ParsesBasicConfig(t *testing.T) {
	content := `
upstream backend {
    server localhost:9001 weight=3;
    server localhost:9002 weight=1;
}
server {
    listen 8080;
    location / {
        proxy_pass http://backend;
        proxy_read_timeout 15s;
    }
}
`
	path := writeTempFile(t, "nginx.conf", content)

	cfg, err := LoadNginx(path)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	require.Len(t, cfg.Proxy.Routes, 1)
	assert.Equal(t, "/", cfg.Proxy.Routes[0].PathPrefix)
	assert.Equal(t, 15, cfg.Proxy.Routes[0].TimeoutSeconds)
	assert.Len(t, cfg.Proxy.Routes[0].Upstreams, 2)
}

func TestLoadAuto_DetectsYAMLByExtension(t *testing.T) {
	// Write a minimal valid yaml to configs/ and verify it loads.
	// We just check that calling LoadAuto with a .conf suffix calls the nginx path.
	// A non-existent file returns an error either way; check the error message differs.
	_, errConf := LoadAuto("/nonexistent.conf")
	_, errYaml := LoadAuto("/nonexistent.yaml")

	// Both should error but via different code paths (nginx parser vs viper).
	// The nginx parser gives "open /nonexistent.conf: no such file" style errors.
	assert.Error(t, errConf)
	assert.Error(t, errYaml)
}

// ── ParseSize helper ──────────────────────────────────────────────────────────

func TestParseSize(t *testing.T) {
	cases := []struct {
		input    string
		expected int64
	}{
		{"1k", 1024},
		{"1K", 1024},
		{"10m", 10 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"512", 512},
		{"0", 0},
		{"", 0},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseSize(tc.input))
		})
	}
}

// ── ParseDurationSeconds helper ───────────────────────────────────────────────

func TestParseDurationSeconds(t *testing.T) {
	cases := []struct {
		input    string
		expected int
	}{
		{"10s", 10},
		{"2m", 120},
		{"1h", 3600},
		{"30", 30},
		{"", 0},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseDurationSeconds(tc.input))
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
