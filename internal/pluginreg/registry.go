// Package pluginreg wires together the plugin interface and all built-in
// plugin implementations. It exists as a separate package to break the
// otherwise circular import between internal/plugin and internal/plugin/plugins.
package pluginreg

import (
	"github.com/krishnaditya65/web-server/internal/plugin"
	"github.com/krishnaditya65/web-server/internal/plugin/plugins"
)

// Default returns a Registry pre-populated with all built-in plugins.
func Default() *plugin.Registry {
	r := plugin.NewRegistry()
	r.Register(plugins.KeyAuthPlugin{})
	r.Register(plugins.JWTPlugin{})
	r.Register(plugins.IPRestrictionPlugin{})
	r.Register(plugins.RequestTransformerPlugin{})
	r.Register(plugins.ResponseTransformerPlugin{})
	r.Register(plugins.RateLimitPlugin{})
	return r
}
