package plugins

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/krishnaditya65/web-server/internal/plugin"
)

// KeyAuthPlugin validates requests against a list of API keys.
// Config keys:
//
//	"header"            string   (default "X-API-Key")
//	"query"             string   (default "api_key")
//	"keys"              []string (required)
//	"hide_credentials"  bool     (strip key before forwarding)
type KeyAuthPlugin struct{}

func (KeyAuthPlugin) Name() string { return "key-auth" }

func (KeyAuthPlugin) New(cfg map[string]interface{}) (plugin.Middleware, error) {
	header := stringOr(cfg, "header", "X-API-Key")
	query := stringOr(cfg, "query", "api_key")
	hideCredentials := boolVal(cfg, "hide_credentials")

	rawKeys, _ := cfg["keys"].([]interface{})
	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("key-auth: 'keys' must be a non-empty list")
	}

	var keys [][]byte
	for _, k := range rawKeys {
		if s, ok := k.(string); ok && s != "" {
			keys = append(keys, []byte(s))
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(header)
			if key == "" {
				key = r.URL.Query().Get(query)
			}

			if !validKey([]byte(key), keys) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if hideCredentials {
				r = r.Clone(r.Context())
				r.Header.Del(header)

				q := r.URL.Query()
				q.Del(query)
				r.URL.RawQuery = q.Encode()
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

func validKey(provided []byte, keys [][]byte) bool {
	for _, k := range keys {
		if subtle.ConstantTimeCompare(provided, k) == 1 {
			return true
		}
	}

	return false
}
