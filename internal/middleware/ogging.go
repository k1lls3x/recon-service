package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type responseWriter struct {
	w      http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) Header() http.Header         { return rw.w.Header() }
func (rw *responseWriter) Write(b []byte) (int, error) { n, err := rw.w.Write(b); rw.size += n; return n, err }
func (rw *responseWriter) WriteHeader(code int)        { rw.status = code; rw.w.WriteHeader(code) }

func Logging(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{w: w, status: 200}
			next.ServeHTTP(rw, r)
			dur := time.Since(start)
			logger.Info().
				Str("rid", GetRequestID(r)).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rw.status).
				Dur("dur", dur).
				Int("size", rw.size).
				Msg("http")
		})
	}
}
