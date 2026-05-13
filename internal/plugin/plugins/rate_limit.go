package plugins

import (
	"fmt"
	"net/http"

	"github.com/krishnaditya65/web-server/internal/middleware"
	"github.com/krishnaditya65/web-server/internal/plugin"
)

// RateLimitPlugin provides per-route, per-IP rate limiting.
// Config keys:
//
//	"requests_per_second"  float64
//	"burst"                int
type RateLimitPlugin struct{}

func (RateLimitPlugin) Name() string { return "rate-limit" }

func (RateLimitPlugin) New(cfg map[string]interface{}) (plugin.Middleware, error) {
	rps, ok := cfg["requests_per_second"].(float64)
	if !ok || rps <= 0 {
		return nil, fmt.Errorf("rate-limit: 'requests_per_second' must be a positive number")
	}

	burst := int(floatVal(cfg, "burst"))
	if burst <= 0 {
		burst = int(rps)
	}

	return func(next http.Handler) http.Handler {
		return middleware.NewIPRateLimit(rps, burst)(next)
	}, nil
}
