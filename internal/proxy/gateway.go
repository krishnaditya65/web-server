package proxy

import (
	"net/http"
	"strings"

	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/metrics"
)

type routeEntry struct {
	name       string
	host       string
	pathPrefix string
	handler    http.Handler
}

type Gateway struct {
	routes []routeEntry
}

func New(cfg *config.Config) http.Handler {
	metricsRegistry := metrics.New()

	var routes []routeEntry

	for _, route := range cfg.Proxy.Routes {
		engine := buildRouteEngine(
			cfg,
			route,
			metricsRegistry,
		)

		routes = append(routes, routeEntry{
			name:       route.Name,
			host:       route.Host,
			pathPrefix: route.PathPrefix,
			handler:    engine,
		})
	}

	return &Gateway{
		routes: routes,
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	best := g.match(r)

	if best == nil {
		http.NotFound(w, r)
		return
	}

	best.handler.ServeHTTP(w, r)
}

func (g *Gateway) match(r *http.Request) *routeEntry {
	var best *routeEntry
	bestScore := -1

	host := r.Host
	path := r.URL.Path

	for i := range g.routes {
		route := &g.routes[i]

		if route.host != "" && !strings.EqualFold(route.host, host) {
			continue
		}

		if !strings.HasPrefix(path, route.pathPrefix) {
			continue
		}

		score := len(route.pathPrefix)

		if route.host != "" {
			score += 10000
		}

		if score > bestScore {
			bestScore = score
			best = route
		}
	}

	return best
}
