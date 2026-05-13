package proxy

import (
	"io"
	"net/http"
	"time"

	"github.com/krishnaditya65/web-server/internal/lb"
	"github.com/krishnaditya65/web-server/internal/metrics"
	"github.com/krishnaditya65/web-server/internal/types"
)

type Engine struct {
	routeName string
	lb        lb.Balancer
	transport http.RoundTripper
	metrics   *metrics.Registry
}

func NewEngine(
	routeName string,
	balancer lb.Balancer,
	transport http.RoundTripper,
	metricsRegistry *metrics.Registry,
) *Engine {
	return &Engine{
		routeName: routeName,
		lb:        balancer,
		transport: transport,
		metrics:   metricsRegistry,
	}
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	target, err := e.lb.Next()
	if err != nil {
		http.Error(w, "no healthy upstreams", http.StatusServiceUnavailable)

		e.metrics.Requests.
			WithLabelValues(
				e.routeName,
				r.Method,
				"503",
				"none",
			).
			Inc()

		e.metrics.Duration.
			WithLabelValues(
				e.routeName,
				r.Method,
				"none",
			).
			Observe(time.Since(start).Seconds())

		return
	}

	// WebSocket / Upgrade path
	if isWebSocketUpgrade(r) {
		upstreamLabel := metrics.UpstreamLabel(target)

		err := e.handleWebSocket(w, r, target)

		if err != nil {
			e.metrics.Failures.
				WithLabelValues(
					e.routeName,
					upstreamLabel,
				).
				Inc()

			if target.FailureCount.Load() == 2 {
				e.metrics.Circuits.
					WithLabelValues(
						e.routeName,
						upstreamLabel,
					).
					Inc()
			}

			e.lb.MarkFailure(target)
		} else {
			target.FailureCount.Store(0)
		}

		e.lb.Release(target)
		e.updateActiveMetrics()

		return
	}

	const maxRetries = 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		upstreamLabel := metrics.UpstreamLabel(target)

		resp, reqErr := e.forward(r, target)

		if reqErr != nil {
			e.metrics.Failures.
				WithLabelValues(
					e.routeName,
					upstreamLabel,
				).
				Inc()

			if target.FailureCount.Load() == 2 {
				e.metrics.Circuits.
					WithLabelValues(
						e.routeName,
						upstreamLabel,
					).
					Inc()
			}

			e.lb.MarkFailure(target)
			e.lb.Release(target)
			e.updateActiveMetrics()

			if shouldRetry(r, nil, reqErr) && attempt < maxRetries {
				e.metrics.Retries.
					WithLabelValues(
						e.routeName,
						upstreamLabel,
					).
					Inc()

				target, err = e.lb.Next()
				if err != nil {
					break
				}

				continue
			}

			http.Error(w, "bad gateway", http.StatusBadGateway)

			e.metrics.Requests.
				WithLabelValues(
					e.routeName,
					r.Method,
					"502",
					upstreamLabel,
				).
				Inc()

			e.metrics.Duration.
				WithLabelValues(
					e.routeName,
					r.Method,
					upstreamLabel,
				).
				Observe(time.Since(start).Seconds())

			return
		}

		if shouldRetry(r, resp, nil) && attempt < maxRetries {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			e.metrics.Failures.
				WithLabelValues(
					e.routeName,
					upstreamLabel,
				).
				Inc()

			if target.FailureCount.Load() == 2 {
				e.metrics.Circuits.
					WithLabelValues(
						e.routeName,
						upstreamLabel,
					).
					Inc()
			}

			e.lb.MarkFailure(target)
			e.lb.Release(target)
			e.updateActiveMetrics()

			e.metrics.Retries.
				WithLabelValues(
					e.routeName,
					upstreamLabel,
				).
				Inc()

			target, err = e.lb.Next()
			if err != nil {
				break
			}

			continue
		}

		target.FailureCount.Store(0)

		err = writeResponse(w, resp)

		e.lb.Release(target)
		e.updateActiveMetrics()

		e.metrics.Requests.
			WithLabelValues(
				e.routeName,
				r.Method,
				metrics.StatusLabel(resp.StatusCode),
				upstreamLabel,
			).
			Inc()

		e.metrics.Duration.
			WithLabelValues(
				e.routeName,
				r.Method,
				upstreamLabel,
			).
			Observe(time.Since(start).Seconds())

		if err != nil {
			return
		}

		return
	}

	http.Error(w, "bad gateway", http.StatusBadGateway)

	e.metrics.Requests.
		WithLabelValues(
			e.routeName,
			r.Method,
			"502",
			"none",
		).
		Inc()

	e.metrics.Duration.
		WithLabelValues(
			e.routeName,
			r.Method,
			"none",
		).
		Observe(time.Since(start).Seconds())
}

func (e *Engine) forward(
	in *http.Request,
	target *types.Upstream,
) (*http.Response, error) {
	out := cloneRequest(in.Context(), in)

	rewriteRequest(out, target)
	prepareOutboundRequest(out)

	return e.transport.RoundTrip(out)
}

func (e *Engine) updateActiveMetrics() {
	for _, up := range e.lb.Upstreams() {
		e.metrics.Active.
			WithLabelValues(
				e.routeName,
				metrics.UpstreamLabel(up),
			).
			Set(float64(up.ActiveConns.Load()))
	}
}
