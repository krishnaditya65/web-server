package middleware

import (
	"net/http"

	"github.com/google/uuid"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()

		w.Header().Set("X-Request-ID", id)
		r.Header.Set("X-Request-ID", id)

		next.ServeHTTP(w, r)
	})
}
