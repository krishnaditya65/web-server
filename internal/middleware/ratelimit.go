package middleware

import (
	"net/http"

	"golang.org/x/time/rate"

	"github.com/krishnaditya65/web-server/internal/config"
)

func RateLimit(cfg *config.Config) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(
		rate.Limit(cfg.Rate.RequestsPerSecond),
		cfg.Rate.Burst,
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
