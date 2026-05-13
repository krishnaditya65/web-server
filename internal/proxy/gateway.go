package proxy

import (
	"net/http"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/krishnaditya65/web-server/internal/admin"
	"github.com/krishnaditya65/web-server/internal/config"
	"github.com/krishnaditya65/web-server/internal/metrics"
	"github.com/krishnaditya65/web-server/internal/plugin"
)

type routeEntry struct {
	name       string
	host       string
	pathPrefix string
	handler    http.Handler
	pluginNames []string // for admin inspection
}

// Gateway routes requests to the correct proxy engine based on host + path.
type Gateway struct {
	routes    atomic.Pointer[[]routeEntry]
	pluginReg *plugin.Registry
	metrics   *metrics.Registry
	logger    *zap.Logger
}

// New builds a Gateway from config. Returns *Gateway (not http.Handler) so
// callers can call Reload and Routes for hot-reload and admin inspection.
func New(cfg *config.Config, pluginReg *plugin.Registry, logger *zap.Logger) *Gateway {
	reg := metrics.New()

	gw := &Gateway{
		pluginReg: pluginReg,
		metrics:   reg,
		logger:    logger,
	}

	routes := gw.buildRoutes(cfg)
	gw.routes.Store(&routes)

	return gw
}

// Reload atomically replaces the route table with one built from newCfg.
func (g *Gateway) Reload(cfg *config.Config) error {
	routes := g.buildRoutes(cfg)
	g.routes.Store(&routes)
	return nil
}

// Routes returns a snapshot of all current routes for admin inspection.
func (g *Gateway) Routes() []admin.RouteSnapshot {
	entries := *g.routes.Load()
	out := make([]admin.RouteSnapshot, 0, len(entries))

	for _, e := range entries {
		out = append(out, admin.RouteSnapshot{
			Name:       e.name,
			PathPrefix: e.pathPrefix,
			Host:       e.host,
			Plugins:    e.pluginNames,
		})
	}

	return out
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries := *g.routes.Load()
	best := g.match(entries, r)

	if best == nil {
		http.NotFound(w, r)
		return
	}

	best.handler.ServeHTTP(w, r)
}

func (g *Gateway) match(entries []routeEntry, r *http.Request) *routeEntry {
	var best *routeEntry
	bestScore := -1

	host := r.Host
	path := r.URL.Path

	for i := range entries {
		route := &entries[i]

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

func (g *Gateway) buildRoutes(cfg *config.Config) []routeEntry {
	var routes []routeEntry

	for _, route := range cfg.Proxy.Routes {
		engine := buildRouteEngine(cfg, route, g.metrics)
		handler := g.buildPluginChain(engine, route.Plugins)
		pluginNames := enabledPluginNames(route.Plugins)

		routes = append(routes, routeEntry{
			name:        route.Name,
			host:        route.Host,
			pathPrefix:  route.PathPrefix,
			handler:     handler,
			pluginNames: pluginNames,
		})
	}

	return routes
}

// buildPluginChain wraps engine with the route's plugin middleware (outermost first).
func (g *Gateway) buildPluginChain(engine http.Handler, pluginCfgs []config.PluginConfig) http.Handler {
	handler := engine

	// Apply in reverse so first plugin in config is outermost (runs first).
	for i := len(pluginCfgs) - 1; i >= 0; i-- {
		pc := pluginCfgs[i]

		if !pc.Enabled {
			continue
		}

		p, ok := g.pluginReg.Get(pc.Name)
		if !ok {
			g.logger.Warn("unknown plugin, skipping", zap.String("plugin", pc.Name))
			continue
		}

		mw, err := p.New(pc.Config)
		if err != nil {
			g.logger.Warn("plugin init failed, skipping",
				zap.String("plugin", pc.Name),
				zap.Error(err),
			)
			continue
		}

		handler = mw(handler)
	}

	return handler
}

func enabledPluginNames(cfgs []config.PluginConfig) []string {
	var names []string

	for _, pc := range cfgs {
		if pc.Enabled {
			names = append(names, pc.Name)
		}
	}

	return names
}
