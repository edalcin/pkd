package server

import (
	"context"
	"net/http"

	"github.com/edalcin/pkd/internal/sessions"
)

type contextKey int

const sessionKey contextKey = iota

const sessionCookieName = "pkd_session"

// AuthRequired is middleware that enforces authentication for API routes.
// On success it injects the *sessions.Session into the request context.
// On failure it returns 401 for /api/* routes and redirects to /login for HTML routes.
func AuthRequired(store *sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				respondUnauth(w, r)
				return
			}
			sess, ok := store.Get(cookie.Value)
			if !ok {
				respondUnauth(w, r)
				return
			}
			store.Touch(sess.ID)
			ctx := context.WithValue(r.Context(), sessionKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionFromContext returns the session stored in the context, or nil.
func SessionFromContext(ctx context.Context) *sessions.Session {
	sess, _ := ctx.Value(sessionKey).(*sessions.Session)
	return sess
}

func respondUnauth(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func isAPIPath(path string) bool {
	return len(path) >= 4 && path[:4] == "/api"
}
