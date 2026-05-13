package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/krishnaditya65/web-server/internal/config"
)

// Gzip returns a middleware that compresses responses with gzip when the client
// supports it and the response content-type matches the configured list.
func Gzip(cfg config.GzipConfig) func(http.Handler) http.Handler {
	pool := &sync.Pool{
		New: func() interface{} {
			w, _ := gzip.NewWriterLevel(io.Discard, cfg.Level)
			return w
		},
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !acceptsGzip(r) {
				next.ServeHTTP(w, r)
				return
			}

			grw := &gzipResponseWriter{
				ResponseWriter: w,
				pool:           pool,
				cfg:            cfg,
				buf:            make([]byte, 0, cfg.MinLength),
			}
			defer grw.close()

			next.ServeHTTP(grw, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	pool        *sync.Pool
	cfg         config.GzipConfig
	gz          *gzip.Writer
	buf         []byte
	status      int
	activated   bool
	headersSent bool
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	g.status = status
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if g.activated {
		return g.gz.Write(b)
	}

	// Buffer until we have enough bytes to decide whether to compress.
	g.buf = append(g.buf, b...)

	if len(g.buf) < g.cfg.MinLength {
		return len(b), nil
	}

	g.activate()
	return len(b), nil
}

func (g *gzipResponseWriter) activate() {
	if g.activated {
		return
	}

	g.activated = true

	ct := g.ResponseWriter.Header().Get("Content-Type")

	if g.matchContentType(ct) {
		gz := g.pool.Get().(*gzip.Writer)
		gz.Reset(g.ResponseWriter)
		g.gz = gz

		h := g.ResponseWriter.Header()
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		h.Add("Vary", "Accept-Encoding")
	}

	g.flushHeaders()

	if g.gz != nil {
		g.gz.Write(g.buf)
	} else {
		g.ResponseWriter.Write(g.buf)
	}

	g.buf = nil
}

func (g *gzipResponseWriter) flushHeaders() {
	if g.headersSent {
		return
	}

	g.headersSent = true

	if g.status != 0 {
		g.ResponseWriter.WriteHeader(g.status)
	}
}

func (g *gzipResponseWriter) close() {
	if !g.activated {
		// Flush any buffered bytes uncompressed (body was shorter than MinLength).
		g.flushHeaders()
		if len(g.buf) > 0 {
			g.ResponseWriter.Write(g.buf)
		}
		return
	}

	if g.gz != nil {
		g.gz.Close()
		g.pool.Put(g.gz)
		g.gz = nil
	}
}

func (g *gzipResponseWriter) matchContentType(ct string) bool {
	if ct == "" {
		return false
	}

	// Strip parameters (e.g. "text/html; charset=utf-8" → "text/html").
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}

	for _, allowed := range g.cfg.ContentTypes {
		if strings.EqualFold(ct, allowed) {
			return true
		}

		// Support wildcard prefix like "text/*".
		if strings.HasSuffix(allowed, "/*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(strings.ToLower(ct), strings.ToLower(prefix)) {
				return true
			}
		}
	}

	return false
}

func acceptsGzip(r *http.Request) bool {
	ae := r.Header.Get("Accept-Encoding")
	for _, token := range strings.Split(ae, ",") {
		token = strings.TrimSpace(token)
		if strings.EqualFold(token, "gzip") ||
			strings.HasPrefix(strings.ToLower(token), "gzip;") {
			return true
		}
	}

	return false
}
