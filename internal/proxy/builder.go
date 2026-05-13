package proxy

import (
	"net/url"
	"time"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/health"
	"github.com/krishnaditya65/web-server/internal/lb"
	"github.com/krishnaditya65/web-server/internal/metrics"
	"github.com/krishnaditya65/web-server/internal/transport"
	"github.com/krishnaditya65/web-server/internal/types"
)

func buildRouteEngine(
	cfg *config.Config,
	route config.RouteConfig,
	metricsRegistry *metrics.Registry,
) *Engine {
	cbThreshold := intOr(route.CircuitBreaker.FailureThreshold, 3)
	cbDuration := time.Duration(intOr(route.CircuitBreaker.OpenDurationSeconds, 30)) * time.Second
	cbHalfOpen := intOr(route.CircuitBreaker.HalfOpenRequests, 1)

	var upstreams []*types.Upstream

	for _, raw := range route.Upstreams {
		parsed, err := url.Parse(raw.URL)
		if err != nil {
			continue
		}

		up := &types.Upstream{
			URL:                parsed,
			Weight:             raw.Weight,
			CBFailureThreshold: cbThreshold,
			CBOpenDuration:     cbDuration,
			CBHalfOpenRequests: cbHalfOpen,
		}

		up.Healthy.Store(true)
		upstreams = append(upstreams, up)
	}

	health.StartHealthChecks(
		upstreams,
		cfg.Health.Path,
		time.Duration(cfg.Health.IntervalSeconds)*time.Second,
		time.Duration(cfg.Health.TimeoutSeconds)*time.Second,
	)

	var balancer lb.Balancer

	switch cfg.Proxy.Algorithm {
	case "least_conn":
		balancer = lb.NewLeastConnections(upstreams)
	case "weighted_rr":
		balancer = lb.NewWeightedRoundRobin(upstreams)
	default:
		balancer = lb.NewRoundRobin(upstreams)
	}

	maxRetries := intOr(route.MaxRetries, 2)
	timeout := time.Duration(intOr(route.TimeoutSeconds, 30)) * time.Second

	return NewEngine(
		route.Name,
		balancer,
		transport.New(),
		metricsRegistry,
		maxRetries,
		route.MaxBodyBytes,
		timeout,
	)
}

func intOr(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
