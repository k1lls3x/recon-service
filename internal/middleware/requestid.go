package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey int

const requestIDKey ctxKey = 1

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get("X-Request-ID")
			if rid == "" {
				rid = uuid.NewString()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, rid)
			w.Header().Set("X-Request-ID", rid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetRequestID(r *http.Request) string {
	if v := r.Context().Value(requestIDKey); v != nil {
		return v.(string)
	}
	return ""
}
