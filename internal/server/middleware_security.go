package server

import "net/http"

// SecurityHeaders adds the standard hardening headers to every response.
// Two CSP policies are exposed: SPA (authenticated routes) and public share.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "interest-cohort=()")
		// HSTS: 1 year, include subdomains — only meaningful behind HTTPS
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		// Default to the SPA CSP; public share routes override this.
		h.Set("Content-Security-Policy", cspSPA)
		next.ServeHTTP(w, r)
	})
}

// PublicShareCSP overrides the default CSP with the stricter public-share policy.
// It is applied per-route by wrapping the public share handler.
func PublicShareCSP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspPublicShare)
		next.ServeHTTP(w, r)
	})
}

const cspSPA = "default-src 'self'; " +
	"script-src 'self'; " +
	"img-src 'self' data: blob: https: http:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"object-src 'self'; " + // allows <embed> for inline PDF preview
	"frame-src 'self'; " + // allows <iframe> as PDF fallback
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

const cspPublicShare = "default-src 'none'; " +
	"script-src 'none'; " +
	"img-src 'self' data: https: http:; " +
	"style-src 'unsafe-inline' https://unpkg.com; " +
	"font-src https://unpkg.com; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'"
