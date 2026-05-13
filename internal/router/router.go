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
	"github.com/krishnaditya65/web-server/internal/pluginreg"
	"github.com/krishnaditya65/web-server/internal/proxy"
)

// New builds the main HTTP handler and returns the chi router alongside the
// Gateway so callers can hold a reference for hot-reload.
func New(cfg *config.Config, logger *zap.Logger) (http.Handler, *proxy.Gateway) {
	pluginReg := pluginreg.Default()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(middleware.Logging(logger))
	r.Use(middleware.RateLimit(cfg))
	r.Use(middleware.CORS)

	if cfg.Gzip.Enabled {
		r.Use(middleware.Gzip(cfg.Gzip))
	}

	// Apply global plugins (configured at top-level in config) to all routes.
	for _, pc := range cfg.Plugins {
		if !pc.Enabled {
			continue
		}

		p, ok := pluginReg.Get(pc.Name)
		if !ok {
			logger.Warn("global plugin not found, skipping", zap.String("plugin", pc.Name))
			continue
		}

		mw, err := p.New(pc.Config)
		if err != nil {
			logger.Warn("global plugin init failed, skipping",
				zap.String("plugin", pc.Name),
				zap.Error(err),
			)
			continue
		}

		r.Use(mw)
	}

	r.Get("/health", health.Handler)
	r.Handle("/metrics", promhttp.Handler())

	gw := proxy.New(cfg, pluginReg, logger)
	r.Handle("/*", gw)

	return r, gw
}
