package proxy

import (
	"errors"
	"net"
	"net/http"
	"syscall"
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
