package plugins

import (
	"net/http"

	"github.com/krishnaditya65/web-server/internal/plugin"
)

// ResponseTransformerPlugin modifies response headers after the upstream responds.
// Config keys:
//
//	"add_headers"    map[string]string
//	"remove_headers" []string
//	"rename_headers" map[string]string  (old name → new name)
type ResponseTransformerPlugin struct{}

func (ResponseTransformerPlugin) Name() string { return "response-header-transformer" }

func (ResponseTransformerPlugin) New(cfg map[string]interface{}) (plugin.Middleware, error) {
	addHeaders := stringMap(cfg, "add_headers")
	removeHeaders := stringList(cfg, "remove_headers")
	renameHeaders := stringMap(cfg, "rename_headers")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := newTransformResponseWriter(w, addHeaders, removeHeaders, renameHeaders)
			next.ServeHTTP(rw, r)
		})
	}, nil
}

type transformResponseWriter struct {
	http.ResponseWriter
	addHeaders    map[string]string
	removeHeaders []string
	renameHeaders map[string]string
	wroteHeader   bool
}

func newTransformResponseWriter(
	w http.ResponseWriter,
	add map[string]string,
	remove []string,
	rename map[string]string,
) *transformResponseWriter {
	return &transformResponseWriter{
		ResponseWriter: w,
		addHeaders:     add,
		removeHeaders:  remove,
		renameHeaders:  rename,
	}
}

func (t *transformResponseWriter) WriteHeader(status int) {
	if !t.wroteHeader {
		t.wroteHeader = true
		t.applyTransforms()
	}

	t.ResponseWriter.WriteHeader(status)
}

func (t *transformResponseWriter) Write(b []byte) (int, error) {
	if !t.wroteHeader {
		t.wroteHeader = true
		t.applyTransforms()
	}

	return t.ResponseWriter.Write(b)
}

func (t *transformResponseWriter) applyTransforms() {
	h := t.ResponseWriter.Header()

	for _, k := range t.removeHeaders {
		h.Del(k)
	}

	for old, newName := range t.renameHeaders {
		if val := h.Get(old); val != "" {
			h.Set(newName, val)
			h.Del(old)
		}
	}

	for k, v := range t.addHeaders {
		h.Set(k, v)
	}
}
