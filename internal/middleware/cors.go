package middleware

import "net/http"

func CORS(allowOrigins []string) func(http.Handler) http.Handler {
	allowAll := len(allowOrigins) == 1 && allowOrigins[0] == "*"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin := r.Header.Get("Origin"); origin != "" {
				for _, o := range allowOrigins {
					if o == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						break
					}
				}
			}
			w.Header().Set("Access-Control-Allow-Headers", "*, Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
