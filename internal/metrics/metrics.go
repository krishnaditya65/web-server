package metrics

import (
	"net/url"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/krishnaditya65/web-server/internal/types"
)

type Registry struct {
	Requests *prometheus.CounterVec
	Duration *prometheus.HistogramVec
	Failures *prometheus.CounterVec
	Retries  *prometheus.CounterVec
	Circuits *prometheus.CounterVec
	Active   *prometheus.GaugeVec
}

func New() *Registry {
	// Use a fresh isolated registry per instance so tests can create multiple
	// Registries without "duplicate collector" panics from the global registry.
	reg := prometheus.NewRegistry()

	m := &Registry{
		Requests: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "gateway_requests_total", Help: "Total gateway requests"},
			[]string{"route", "method", "status", "upstream"},
		),
		Duration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "gateway_request_duration_seconds", Help: "Gateway request latency"},
			[]string{"route", "method", "upstream"},
		),
		Failures: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "gateway_upstream_failures_total", Help: "Total upstream failures"},
			[]string{"route", "upstream"},
		),
		Retries: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "gateway_retries_total", Help: "Total retry attempts"},
			[]string{"route", "upstream"},
		),
		Circuits: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "gateway_circuit_breaker_open_total", Help: "Circuit breaker openings"},
			[]string{"route", "upstream"},
		),
		Active: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "gateway_active_connections", Help: "Current active upstream connections"},
			[]string{"route", "upstream"},
		),
	}

	reg.MustRegister(m.Requests, m.Duration, m.Failures, m.Retries, m.Circuits, m.Active)

	// Also register with the global default registry so /metrics endpoint works
	// in production. Ignore "already registered" errors gracefully.
	registerGlobal(m.Requests, m.Duration, m.Failures, m.Retries, m.Circuits, m.Active)

	return m
}

func registerGlobal(cs ...prometheus.Collector) {
	for _, c := range cs {
		_ = prometheus.Register(c) // silently ignore duplicate registration
	}
}

func UpstreamLabel(up *types.Upstream) string {
	return canonical(up.URL)
}

func canonical(u *url.URL) string {
	if u == nil {
		return "unknown"
	}

	return u.Host
}

func StatusLabel(code int) string {
	return strconv.Itoa(code)
}
