package plugin

import "net/http"

// Middleware is a standard Go HTTP middleware function.
type Middleware = func(http.Handler) http.Handler

// Plugin is a named middleware factory. Instantiated once per route with its config.
type Plugin interface {
	Name() string
	New(cfg map[string]interface{}) (Middleware, error)
}

// Registry holds plugin factories keyed by name.
type Registry struct {
	plugins map[string]Plugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin)}
}

func (r *Registry) Register(p Plugin) {
	r.plugins[p.Name()] = p
}

func (r *Registry) Get(name string) (Plugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

