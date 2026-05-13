package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/krishnaditya65/web-server/internal/config"
)

// RateLimit returns a per-source-IP rate limiter middleware using the global config.
func RateLimit(cfg *config.Config) func(http.Handler) http.Handler {
	return NewIPRateLimit(cfg.Rate.RequestsPerSecond, cfg.Rate.Burst)
}

// NewIPRateLimit creates a per-IP token-bucket rate limiter.
// A background goroutine evicts entries not seen in the last 10 minutes.
func NewIPRateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	store := &ipStore{
		ips:   make(map[string]*ipEntry),
		rps:   rate.Limit(rps),
		burst: burst,
	}

	go store.cleanup()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			lim := store.get(ip)

			if !lim.Allow() {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipStore struct {
	mu    sync.Mutex
	ips   map[string]*ipEntry
	rps   rate.Limit
	burst int
}

func (s *ipStore) get(ip string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.ips[ip]
	if !ok {
		e = &ipEntry{limiter: rate.NewLimiter(s.rps, s.burst)}
		s.ips[ip] = e
	}

	e.lastSeen = time.Now()

	return e.limiter
}

func (s *ipStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)

		s.mu.Lock()
		for ip, e := range s.ips {
			if e.lastSeen.Before(cutoff) {
				delete(s.ips, ip)
			}
		}
		s.mu.Unlock()
	}
}

// clientIP extracts the real client IP from the request.
// Prefers X-Forwarded-For (first entry), falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
}
