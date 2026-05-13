package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

func Logging(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			recorder := NewResponseRecorder(w)

			next.ServeHTTP(recorder, r)

			logger.Info("request completed",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", recorder.StatusCode),
				zap.Int("bytes", recorder.Bytes),
				zap.Duration("latency", time.Since(start)),
				zap.String("request_id", r.Header.Get("X-Request-ID")),
				zap.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}
