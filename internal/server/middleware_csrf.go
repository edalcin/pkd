package server

import (
	"net/http"
	"strings"
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
		// Bearer token requests (server-to-server API calls) are not subject to
		// CSRF because they cannot be triggered by a cross-origin page passively
		// — the caller must explicitly know and set the token.
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			ensureCSRFCookie(w, r)
			next.ServeHTTP(w, r)
		default:
			if isShareTargetPost(r) {
				next.ServeHTTP(w, r)
				return
			}
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

// isShareTargetPost reports whether r is the PWA share_target form POST.
// Android starts it from the share sheet, so it cannot carry X-CSRF-Token.
// Sec-Fetch-Site "none" marks a browser-initiated request, never a
// cross-site page; and the SameSite=Strict session cookie is not sent on
// cross-site requests anyway, so AuthRequired still rejects forged posts.
func isShareTargetPost(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return r.URL.Path == "/api/capture" &&
		strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") &&
		(site == "none" || site == "same-origin")
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
