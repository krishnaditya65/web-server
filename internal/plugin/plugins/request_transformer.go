package plugins

import (
	"net/http"

	"github.com/krishnaditya65/web-server/internal/plugin"
)

// RequestTransformerPlugin modifies request headers and query params before forwarding.
// Config keys:
//
//	"add_headers"    map[string]string
//	"remove_headers" []string
//	"rename_headers" map[string]string  (old name → new name)
//	"add_query"      map[string]string
//	"remove_query"   []string
type RequestTransformerPlugin struct{}

func (RequestTransformerPlugin) Name() string { return "request-header-transformer" }

func (RequestTransformerPlugin) New(cfg map[string]interface{}) (plugin.Middleware, error) {
	addHeaders := stringMap(cfg, "add_headers")
	removeHeaders := stringList(cfg, "remove_headers")
	renameHeaders := stringMap(cfg, "rename_headers")
	addQuery := stringMap(cfg, "add_query")
	removeQuery := stringList(cfg, "remove_query")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.Clone(r.Context())

			for _, h := range removeHeaders {
				r.Header.Del(h)
			}

			for old, newName := range renameHeaders {
				if val := r.Header.Get(old); val != "" {
					r.Header.Set(newName, val)
					r.Header.Del(old)
				}
			}

			for k, v := range addHeaders {
				r.Header.Set(k, v)
			}

			if len(addQuery) > 0 || len(removeQuery) > 0 {
				q := r.URL.Query()

				for _, k := range removeQuery {
					q.Del(k)
				}

				for k, v := range addQuery {
					q.Set(k, v)
				}

				r.URL.RawQuery = q.Encode()
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
