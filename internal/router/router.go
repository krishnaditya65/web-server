package router

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/health"
	"github.com/krishnaditya65/web-server/internal/middleware"
	"github.com/krishnaditya65/web-server/internal/proxy"
)

func New(cfg *config.Config, logger *zap.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.Logging(logger))
	r.Use(middleware.RateLimit(cfg))
	r.Use(middleware.CORS)

	r.Get("/health", health.Handler)
	r.Handle("/metrics", promhttp.Handler())

	p := proxy.New(cfg)

	r.Handle("/*", p)

	return r
}
