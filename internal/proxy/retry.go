package proxy

import (
	"errors"
	"math"
	"math/rand"
	"net"
	"net/http"
	"syscall"
	"time"
)

func isReplaySafeMethod(method string) bool {
	switch method {
	case http.MethodGet:
		return true
	case http.MethodHead:
		return true
	case http.MethodOptions:
		return true
	default:
		return false
	}
}

func shouldRetry(req *http.Request, resp *http.Response, err error) bool {
	if !isReplaySafeMethod(req.Method) {
		return false
	}

	if err != nil {
		return isRetryableTransportError(err)
	}

	if resp == nil {
		return false
	}

	switch resp.StatusCode {
	case http.StatusBadGateway: // 502
		return true
	case http.StatusServiceUnavailable: // 503
		return true
	case http.StatusGatewayTimeout: // 504
		return true
	default:
		return false
	}
}

func isRetryableTransportError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	if errors.Is(err, syscall.EPIPE) {
		return true
	}

	return false
}

// RetryDelay returns a full-jitter exponential backoff duration for attempt N (0-indexed).
// Formula: random(0, min(1s, 50ms * 2^attempt))
func RetryDelay(attempt int) time.Duration {
	base := float64(50 * time.Millisecond)
	cap := float64(time.Second)
	ceiling := math.Min(cap, base*math.Pow(2, float64(attempt)))
	jitter := rand.Float64() * ceiling
	return time.Duration(jitter)
}
