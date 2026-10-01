package server

import (
	"context"
	"net/http"
	"strings"
)

// contextKey for request-scoped values.
type contextKey string

const apiKeyContextKey contextKey = "api_key"

// PublicPaths never require auth.
var publicPaths = map[string]bool{
	"/":       true,
	"/health": true,
}

// APIAuthMiddleware validates API key on /api/* paths.
func APIAuthMiddleware(settings *Settings) func(http.Handler) http.Handler {
	allKeys := settings.AllKeys()
	enabled := settings.AuthEnabled()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			path := r.URL.Path
			if publicPaths[path] || !strings.HasPrefix(path, "/api") {
				next.ServeHTTP(w, r)
				return
			}

			key := extractKey(r)
			if !allKeys[key] {
				http.Error(w, `{"error":"invalid or missing API key"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), apiKeyContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractKey(r *http.Request) string {
	// X-API-Key header
	if k := r.Header.Get("X-API-Key"); k != "" {
		return strings.TrimSpace(k)
	}
	// Authorization: Bearer <key>
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	// ?api_key=<key>
	if k := r.URL.Query().Get("api_key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}