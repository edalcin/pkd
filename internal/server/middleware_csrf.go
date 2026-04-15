package server

import (
	"net/http"
	"time"

	"github.com/edalcin/pkd/internal/security"
)

const (
	csrfCookieName = "pkd_csrf"
	csrfHeaderName = "X-CSRF-Token"
)

// CSRF implements the double-submit cookie pattern.
//
// On GET/HEAD/OPTIONS: if no pkd_csrf cookie exists, a new token is set.
// On mutating methods: the X-CSRF-Token header must match the cookie value.
// A mismatch returns 403.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			ensureCSRFCookie(w, r)
			next.ServeHTTP(w, r)
		default:
			cookie, err := r.Cookie(csrfCookieName)
			if err != nil {
				http.Error(w, "missing CSRF cookie", http.StatusForbidden)
				return
			}
			header := r.Header.Get(csrfHeaderName)
			if !security.ConstantTimeEqual(cookie.Value, header) {
				http.Error(w, "CSRF token mismatch", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(csrfCookieName); err == nil {
		return // already set
	}
	token := security.NewCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // must be readable by JS to put it in the header
		Secure:   false, // set to true in prod via reverse proxy TLS
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
}
