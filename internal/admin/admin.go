package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/krishnaditya65/web-server/internal/config"
)

// Reloadable is satisfied by *proxy.Gateway (duck-typed — no import cycle).
type Reloadable interface {
	Reload(cfg *config.Config) error
	Routes() []RouteSnapshot
}

// RouteSnapshot and UpstreamSnapshot mirror proxy.RouteSnapshot / UpstreamSnapshot
// but live here to avoid an admin → proxy import cycle.
type RouteSnapshot struct {
	Name       string             `json:"name"`
	PathPrefix string             `json:"path_prefix"`
	Host       string             `json:"host,omitempty"`
	Plugins    []string           `json:"plugins,omitempty"`
	Upstreams  []UpstreamSnapshot `json:"upstreams,omitempty"`
}

type UpstreamSnapshot struct {
	URL          string `json:"url"`
	Healthy      bool   `json:"healthy"`
	ActiveConns  int64  `json:"active_conns"`
	CircuitState string `json:"circuit_state"`
	FailureCount int64  `json:"failure_count"`
}

// Server is the admin HTTP API server.
type Server struct {
	gw      Reloadable
	cfg     *config.Config
	httpSrv *http.Server
	logger  *zap.Logger
}

func New(cfg *config.Config, gw Reloadable, logger *zap.Logger) *Server {
	s := &Server{gw: gw, cfg: cfg, logger: logger}

	r := chi.NewRouter()
	r.Get("/admin/routes", s.handleRoutes)
	r.Get("/admin/routes/{name}", s.handleRoute)
	r.Get("/admin/upstreams", s.handleUpstreams)
	r.Post("/admin/reload", s.handleReload)
	r.Get("/admin/health", s.handleHealth)

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Admin.Host, cfg.Admin.Port),
		Handler: r,
	}

	return s
}

func (s *Server) Start() error {
	s.logger.Info("admin server starting", zap.String("addr", s.httpSrv.Addr))

	err := s.httpSrv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	snapshots := s.gw.Routes()
	writeJSON(w, snapshots)
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	snapshots := s.gw.Routes()

	for _, snap := range snapshots {
		if snap.Name == name {
			writeJSON(w, snap)
			return
		}
	}

	http.Error(w, "route not found", http.StatusNotFound)
}

func (s *Server) handleUpstreams(w http.ResponseWriter, r *http.Request) {
	// Collect all upstreams across all routes from snapshots.
	snapshots := s.gw.Routes()

	var all []map[string]interface{}
	for _, route := range snapshots {
		for _, up := range route.Upstreams {
			all = append(all, map[string]interface{}{
				"route":         route.Name,
				"url":           up.URL,
				"healthy":       up.Healthy,
				"active_conns":  up.ActiveConns,
				"circuit_state": up.CircuitState,
				"failure_count": up.FailureCount,
			})
		}
	}

	writeJSON(w, all)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	newCfg, err := config.Reload()
	if err != nil {
		s.logger.Error("admin reload: config load failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("config reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	if err := s.gw.Reload(newCfg); err != nil {
		s.logger.Error("admin reload: gateway reload failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("gateway reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.cfg = newCfg
	s.logger.Info("admin reload: config reloaded successfully")

	writeJSON(w, map[string]string{"status": "reloaded"})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if err := enc.Encode(v); err != nil {
		http.Error(w, "json encode error", http.StatusInternalServerError)
	}
}
