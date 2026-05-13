package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishnaditya65/web-server/internal/config"
)

func defaultGzipCfg() config.GzipConfig {
	return config.GzipConfig{
		Enabled:   true,
		Level:     6,
		MinLength: 20, // low threshold so small test bodies trigger compression
		ContentTypes: []string{
			"application/json",
			"text/plain",
			"text/html",
		},
	}
}

// bodyHandler returns a handler that writes body with the given content-type.
func bodyHandler(contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	})
}

func gzipRequest(handler http.Handler, acceptGzip bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ── Compression activated ─────────────────────────────────────────────────────

func TestGzip_CompressesEligibleResponse(t *testing.T) {
	body := strings.Repeat("hello world ", 10) // 120 bytes > MinLength=20
	handler := Gzip(defaultGzipCfg())(bodyHandler("application/json", body))

	rec := gzipRequest(handler, true)

	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")

	// Decompress and verify body.
	r, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer r.Close()
	decoded, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, body, string(decoded))
}

// ── No compression when client does not accept gzip ───────────────────────────

func TestGzip_NoCompressionWithoutAcceptHeader(t *testing.T) {
	body := strings.Repeat("hello world ", 10)
	handler := Gzip(defaultGzipCfg())(bodyHandler("application/json", body))

	rec := gzipRequest(handler, false)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, body, rec.Body.String())
}

// ── Body shorter than MinLength is not compressed ─────────────────────────────

func TestGzip_NoCompressionBelowMinLength(t *testing.T) {
	cfg := defaultGzipCfg()
	cfg.MinLength = 1000 // require 1000 bytes

	body := "short"
	handler := Gzip(cfg)(bodyHandler("application/json", body))

	rec := gzipRequest(handler, true)

	assert.Empty(t, rec.Header().Get("Content-Encoding"), "short body must not be compressed")
	assert.Equal(t, body, rec.Body.String())
}

// ── Non-matching content-type is not compressed ───────────────────────────────

func TestGzip_NoCompressionForBinaryContentType(t *testing.T) {
	body := strings.Repeat("x", 500)
	handler := Gzip(defaultGzipCfg())(bodyHandler("image/png", body))

	rec := gzipRequest(handler, true)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
}

// ── Content-type with parameters (e.g. charset) ───────────────────────────────

func TestGzip_HandlesContentTypeWithParameters(t *testing.T) {
	body := strings.Repeat("hello ", 30)
	handler := Gzip(defaultGzipCfg())(bodyHandler("text/html; charset=utf-8", body))

	rec := gzipRequest(handler, true)

	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
}

// ── Status code is preserved ──────────────────────────────────────────────────

func TestGzip_PreservesStatusCode(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(strings.Repeat("{}", 20)))
	})

	handler := Gzip(defaultGzipCfg())(inner)
	rec := gzipRequest(handler, true)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

// ── Content-Length is stripped when gzip is active ───────────────────────────

func TestGzip_RemovesContentLengthWhenCompressing(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := strings.Repeat("data", 50)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "200") // upstream sets length
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	})

	handler := Gzip(defaultGzipCfg())(inner)
	rec := gzipRequest(handler, true)

	assert.Empty(t, rec.Header().Get("Content-Length"),
		"Content-Length must be removed since compressed size differs")
}

// ── acceptsGzip helper ────────────────────────────────────────────────────────

func TestAcceptsGzip_Variations(t *testing.T) {
	makeReq := func(ae string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", ae)
		return req
	}

	assert.True(t, acceptsGzip(makeReq("gzip")))
	assert.True(t, acceptsGzip(makeReq("deflate, gzip")))
	assert.True(t, acceptsGzip(makeReq("gzip;q=1.0")))
	assert.False(t, acceptsGzip(makeReq("deflate")))
	assert.False(t, acceptsGzip(makeReq("")))
}
