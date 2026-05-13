package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
})

func send(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	return send(handler, httptest.NewRequest(http.MethodGet, path, nil))
}

// ── KeyAuth ───────────────────────────────────────────────────────────────────

func TestKeyAuth_ValidHeaderKey(t *testing.T) {
	mw, err := KeyAuthPlugin{}.New(map[string]interface{}{
		"header": "X-API-Key",
		"keys":   []interface{}{"secret123"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "secret123")

	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestKeyAuth_InvalidKey(t *testing.T) {
	mw, _ := KeyAuthPlugin{}.New(map[string]interface{}{
		"keys": []interface{}{"secret123"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "wrongkey")

	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestKeyAuth_MissingKey(t *testing.T) {
	mw, _ := KeyAuthPlugin{}.New(map[string]interface{}{
		"keys": []interface{}{"secret123"},
	})

	rec := get(mw(okHandler), "/")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestKeyAuth_QueryParam(t *testing.T) {
	mw, _ := KeyAuthPlugin{}.New(map[string]interface{}{
		"query": "token",
		"keys":  []interface{}{"abc"},
	})

	req := httptest.NewRequest(http.MethodGet, "/?token=abc", nil)
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestKeyAuth_HideCredentials(t *testing.T) {
	var capturedHeader string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	})

	mw, _ := KeyAuthPlugin{}.New(map[string]interface{}{
		"header":           "X-API-Key",
		"keys":             []interface{}{"mykey"},
		"hide_credentials": true,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "mykey")
	send(mw(inner), req)

	assert.Empty(t, capturedHeader, "X-API-Key must be stripped before forwarding")
}

func TestKeyAuth_MultipleValidKeys(t *testing.T) {
	mw, _ := KeyAuthPlugin{}.New(map[string]interface{}{
		"keys": []interface{}{"key1", "key2", "key3"},
	})

	for _, k := range []string{"key1", "key2", "key3"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-API-Key", k)
		rec := send(mw(okHandler), req)
		assert.Equal(t, 200, rec.Code, "key %s should be valid", k)
	}
}

func TestKeyAuth_EmptyKeysErrors(t *testing.T) {
	_, err := KeyAuthPlugin{}.New(map[string]interface{}{
		"keys": []interface{}{},
	})
	assert.Error(t, err)
}

// ── JWT ───────────────────────────────────────────────────────────────────────

func makeJWT(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestJWT_ValidToken(t *testing.T) {
	secret := "test-secret"
	token := makeJWT(t, secret, jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	mw, err := JWTPlugin{}.New(map[string]interface{}{
		"secret":    secret,
		"algorithm": "HS256",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJWT_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	token := makeJWT(t, secret, jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(-time.Hour).Unix(), // expired
	})

	mw, _ := JWTPlugin{}.New(map[string]interface{}{
		"secret":    secret,
		"algorithm": "HS256",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_WrongSignature(t *testing.T) {
	token := makeJWT(t, "correct-secret", jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	mw, _ := JWTPlugin{}.New(map[string]interface{}{
		"secret":    "wrong-secret",
		"algorithm": "HS256",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_MissingAuthorizationHeader(t *testing.T) {
	mw, _ := JWTPlugin{}.New(map[string]interface{}{
		"secret":    "secret",
		"algorithm": "HS256",
	})
	rec := get(mw(okHandler), "/")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_ClaimsInjectedAsHeaders(t *testing.T) {
	secret := "secret"
	token := makeJWT(t, secret, jwt.MapClaims{
		"sub":   "user42",
		"email": "user@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	var capturedUID, capturedEmail string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUID = r.Header.Get("X-User-ID")
		capturedEmail = r.Header.Get("X-User-Email")
		w.WriteHeader(200)
	})

	mw, _ := JWTPlugin{}.New(map[string]interface{}{
		"secret":    secret,
		"algorithm": "HS256",
		"claims_to_headers": map[string]interface{}{
			"sub":   "X-User-ID",
			"email": "X-User-Email",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	send(mw(inner), req)

	assert.Equal(t, "user42", capturedUID)
	assert.Equal(t, "user@example.com", capturedEmail)
}

func TestJWT_EmptySecretErrors(t *testing.T) {
	_, err := JWTPlugin{}.New(map[string]interface{}{
		"secret":    "",
		"algorithm": "HS256",
	})
	assert.Error(t, err)
}

// ── IPRestriction ─────────────────────────────────────────────────────────────

func TestIPRestriction_AllowlistPassesMatchingIP(t *testing.T) {
	mw, err := IPRestrictionPlugin{}.New(map[string]interface{}{
		"allow": []interface{}{"192.168.1.0/24"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:1234"
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIPRestriction_AllowlistBlocksNonMatchingIP(t *testing.T) {
	mw, _ := IPRestrictionPlugin{}.New(map[string]interface{}{
		"allow": []interface{}{"192.168.1.0/24"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestIPRestriction_DenylistBlocksMatchingIP(t *testing.T) {
	mw, _ := IPRestrictionPlugin{}.New(map[string]interface{}{
		"deny": []interface{}{"10.0.0.0/8"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestIPRestriction_DenylistPassesNonMatchingIP(t *testing.T) {
	mw, _ := IPRestrictionPlugin{}.New(map[string]interface{}{
		"deny": []interface{}{"10.0.0.0/8"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIPRestriction_ExactIP(t *testing.T) {
	mw, _ := IPRestrictionPlugin{}.New(map[string]interface{}{
		"allow": []interface{}{"1.2.3.4"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rec := send(mw(okHandler), req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIPRestriction_InvalidCIDRErrors(t *testing.T) {
	_, err := IPRestrictionPlugin{}.New(map[string]interface{}{
		"allow": []interface{}{"not-an-ip"},
	})
	assert.Error(t, err)
}

// ── RequestTransformer ────────────────────────────────────────────────────────

func TestRequestTransformer_AddsHeaders(t *testing.T) {
	var got string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Custom")
		w.WriteHeader(200)
	})

	mw, _ := RequestTransformerPlugin{}.New(map[string]interface{}{
		"add_headers": map[string]interface{}{"X-Custom": "injected"},
	})

	get(mw(inner), "/")
	assert.Equal(t, "injected", got)
}

func TestRequestTransformer_RemovesHeaders(t *testing.T) {
	var got string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Remove-Me")
		w.WriteHeader(200)
	})

	mw, _ := RequestTransformerPlugin{}.New(map[string]interface{}{
		"remove_headers": []interface{}{"X-Remove-Me"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Remove-Me", "should-be-gone")
	send(mw(inner), req)
	assert.Empty(t, got)
}

func TestRequestTransformer_RenamesHeader(t *testing.T) {
	var newVal, oldVal string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldVal = r.Header.Get("X-Old")
		newVal = r.Header.Get("X-New")
		w.WriteHeader(200)
	})

	mw, _ := RequestTransformerPlugin{}.New(map[string]interface{}{
		"rename_headers": map[string]interface{}{"X-Old": "X-New"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Old", "value")
	send(mw(inner), req)
	assert.Empty(t, oldVal, "original header must be removed")
	assert.Equal(t, "value", newVal, "value must be on new header name")
}

// ── ResponseTransformer ───────────────────────────────────────────────────────

func TestResponseTransformer_AddsResponseHeaders(t *testing.T) {
	mw, _ := ResponseTransformerPlugin{}.New(map[string]interface{}{
		"add_headers": map[string]interface{}{"X-Powered-By": "gateway"},
	})

	rec := send(mw(okHandler), httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, "gateway", rec.Header().Get("X-Powered-By"))
}

func TestResponseTransformer_RemovesResponseHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "apache")
		w.WriteHeader(200)
	})

	mw, _ := ResponseTransformerPlugin{}.New(map[string]interface{}{
		"remove_headers": []interface{}{"Server"},
	})

	rec := send(mw(inner), httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Empty(t, rec.Header().Get("Server"))
}

// ── Plugin names ──────────────────────────────────────────────────────────────

func TestPluginNames(t *testing.T) {
	assert.Equal(t, "key-auth", KeyAuthPlugin{}.Name())
	assert.Equal(t, "jwt", JWTPlugin{}.Name())
	assert.Equal(t, "ip-restriction", IPRestrictionPlugin{}.Name())
	assert.Equal(t, "request-header-transformer", RequestTransformerPlugin{}.Name())
	assert.Equal(t, "response-header-transformer", ResponseTransformerPlugin{}.Name())
	assert.Equal(t, "rate-limit", RateLimitPlugin{}.Name())
}
