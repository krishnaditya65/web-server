package plugins

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/krishnaditya65/web-server/internal/plugin"
)

// JWTPlugin validates a Bearer token in the Authorization header.
// Config keys:
//
//	"secret"             string            (HMAC secret for HS256)
//	"algorithm"          string            (default "HS256")
//	"claims_to_headers"  map[string]string (JWT claim → upstream header name)
type JWTPlugin struct{}

func (JWTPlugin) Name() string { return "jwt" }

func (JWTPlugin) New(cfg map[string]interface{}) (plugin.Middleware, error) {
	secret := stringOr(cfg, "secret", "")
	algorithm := stringOr(cfg, "algorithm", "HS256")
	claimsToHeaders := stringMap(cfg, "claims_to_headers")

	if secret == "" {
		return nil, fmt.Errorf("jwt: 'secret' is required")
	}

	var keyFunc jwt.Keyfunc

	switch strings.ToUpper(algorithm) {
	case "HS256", "HS384", "HS512":
		keyFunc = func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}

			return []byte(secret), nil
		}
	default:
		return nil, fmt.Errorf("jwt: unsupported algorithm %q (only HS256/384/512 supported)", algorithm)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(tokenStr, keyFunc,
				jwt.WithValidMethods([]string{algorithm}),
				jwt.WithExpirationRequired(),
			)
			if err != nil || !token.Valid {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}

			if len(claimsToHeaders) > 0 {
				r = r.Clone(r.Context())
				for claim, header := range claimsToHeaders {
					if val, exists := claims[claim]; exists {
						r.Header.Set(header, fmt.Sprintf("%v", val))
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}

	return strings.TrimSpace(auth[len(prefix):])
}
