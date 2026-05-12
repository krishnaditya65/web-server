package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/health"
	"github.com/krishnaditya65/web-server/internal/middleware"
	"github.com/krishnaditya65/web-server/internal/proxy"
)

func New(cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logging)
	r.Use(middleware.Recovery)
	r.Use(middleware.RateLimit(cfg))
	r.Use(middleware.CORS)

	r.Get("/health", health.Handler)
	r.Handle("/metrics", promhttp.Handler())

	p := proxy.New(cfg)
	r.Handle("/*", p)

	return r
}
