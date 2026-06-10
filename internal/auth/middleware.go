package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey int

const userIDKey contextKey = 0

const AnonymousUserID = "anon"

// Middleware returns an HTTP middleware that injects the user ID into the
// request context. When authEnabled is false every request gets the anonymous
// user ID. When true, a valid Bearer token is required or a 401 is returned.
func Middleware(store Store, authEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authEnabled {
				r = r.WithContext(WithUserID(r.Context(), AnonymousUserID))
				next.ServeHTTP(w, r)
				return
			}

			token := extractBearer(r)
			if token == "" {
				http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
				return
			}

			uid, ok := store.Lookup(token)
			if !ok {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			r = r.WithContext(WithUserID(r.Context(), uid))
			next.ServeHTTP(w, r)
		})
	}
}

// WithUserID returns a context carrying the given user ID.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext retrieves the user ID stored by Middleware.
func UserIDFromContext(ctx context.Context) string {
	if uid, ok := ctx.Value(userIDKey).(string); ok {
		return uid
	}
	return AnonymousUserID
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// Also accept token via query param for easy browser testing.
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}
