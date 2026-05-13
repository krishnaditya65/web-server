package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/krishnaditya65/web-server/internal/lb"
	"github.com/krishnaditya65/web-server/internal/metrics"
	"github.com/krishnaditya65/web-server/internal/types"
)

type Engine struct {
	routeName    string
	lb           lb.Balancer
	transport    http.RoundTripper
	metrics      *metrics.Registry
	maxRetries   int
	maxBodyBytes int64
	timeout      time.Duration
}

func NewEngine(
	routeName string,
	balancer lb.Balancer,
	transport http.RoundTripper,
	metricsRegistry *metrics.Registry,
	maxRetries int,
	maxBodyBytes int64,
	timeout time.Duration,
) *Engine {
	return &Engine{
		routeName:    routeName,
		lb:           balancer,
		transport:    transport,
		metrics:      metricsRegistry,
		maxRetries:   maxRetries,
		maxBodyBytes: maxBodyBytes,
		timeout:      timeout,
	}
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Enforce body size limit before touching any upstream.
	if e.maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, e.maxBodyBytes)
	}

	target, err := e.lb.Next()
	if err != nil {
		http.Error(w, "no healthy upstreams", http.StatusServiceUnavailable)

		e.metrics.Requests.WithLabelValues(e.routeName, r.Method, "503", "none").Inc()
		e.metrics.Duration.WithLabelValues(e.routeName, r.Method, "none").Observe(time.Since(start).Seconds())

		return
	}

	// WebSocket / Upgrade path
	if isWebSocketUpgrade(r) {
		upstreamLabel := metrics.UpstreamLabel(target)

		err := e.handleWebSocket(w, r, target)

		if err != nil {
			e.metrics.Failures.WithLabelValues(e.routeName, upstreamLabel).Inc()

			if target.FailureCount.Load() == int64(target.CBFailureThreshold-1) {
				e.metrics.Circuits.WithLabelValues(e.routeName, upstreamLabel).Inc()
			}

			e.lb.MarkFailure(target)
		} else {
			target.RecordSuccess()
		}

		e.lb.Release(target)
		e.updateActiveMetrics()

		return
	}

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		upstreamLabel := metrics.UpstreamLabel(target)

		resp, reqErr := e.forward(r, target)

		if reqErr != nil {
			// Check for body-too-large error before treating as upstream failure.
			var maxBytesErr *http.MaxBytesError
			if isMaxBytesError(reqErr, &maxBytesErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				e.lb.Release(target)
				e.updateActiveMetrics()
				return
			}

			e.metrics.Failures.WithLabelValues(e.routeName, upstreamLabel).Inc()

			if target.FailureCount.Load() == int64(target.CBFailureThreshold-1) {
				e.metrics.Circuits.WithLabelValues(e.routeName, upstreamLabel).Inc()
			}

			e.lb.MarkFailure(target)
			e.lb.Release(target)
			e.updateActiveMetrics()

			if shouldRetry(r, nil, reqErr) && attempt < e.maxRetries {
				e.metrics.Retries.WithLabelValues(e.routeName, upstreamLabel).Inc()

				time.Sleep(RetryDelay(attempt))

				target, err = e.lb.Next()
				if err != nil {
					break
				}

				continue
			}

			http.Error(w, "bad gateway", http.StatusBadGateway)

			e.metrics.Requests.WithLabelValues(e.routeName, r.Method, "502", upstreamLabel).Inc()
			e.metrics.Duration.WithLabelValues(e.routeName, r.Method, upstreamLabel).Observe(time.Since(start).Seconds())

			return
		}

		if shouldRetry(r, resp, nil) && attempt < e.maxRetries {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			e.metrics.Failures.WithLabelValues(e.routeName, upstreamLabel).Inc()

			if target.FailureCount.Load() == int64(target.CBFailureThreshold-1) {
				e.metrics.Circuits.WithLabelValues(e.routeName, upstreamLabel).Inc()
			}

			e.lb.MarkFailure(target)
			e.lb.Release(target)
			e.updateActiveMetrics()

			e.metrics.Retries.WithLabelValues(e.routeName, upstreamLabel).Inc()

			time.Sleep(RetryDelay(attempt))

			target, err = e.lb.Next()
			if err != nil {
				break
			}

			continue
		}

		target.RecordSuccess()

		err = writeResponse(w, resp)

		e.lb.Release(target)
		e.updateActiveMetrics()

		e.metrics.Requests.WithLabelValues(e.routeName, r.Method, metrics.StatusLabel(resp.StatusCode), upstreamLabel).Inc()
		e.metrics.Duration.WithLabelValues(e.routeName, r.Method, upstreamLabel).Observe(time.Since(start).Seconds())

		if err != nil {
			return
		}

		return
	}

	http.Error(w, "bad gateway", http.StatusBadGateway)

	e.metrics.Requests.WithLabelValues(e.routeName, r.Method, "502", "none").Inc()
	e.metrics.Duration.WithLabelValues(e.routeName, r.Method, "none").Observe(time.Since(start).Seconds())
}

func (e *Engine) forward(in *http.Request, target *types.Upstream) (*http.Response, error) {
	ctx := in.Context()

	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	out := cloneRequest(ctx, in)

	rewriteRequest(out, target)
	prepareOutboundRequest(out)

	return e.transport.RoundTrip(out)
}

func (e *Engine) updateActiveMetrics() {
	for _, up := range e.lb.Upstreams() {
		e.metrics.Active.
			WithLabelValues(e.routeName, metrics.UpstreamLabel(up)).
			Set(float64(up.ActiveConns.Load()))
	}
}

// isMaxBytesError checks whether err wraps an *http.MaxBytesError anywhere in its chain.
func isMaxBytesError(err error, target **http.MaxBytesError) bool {
	if err == nil {
		return false
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		if target != nil {
			*target = mbe
		}
		return true
	}
	return false
}
