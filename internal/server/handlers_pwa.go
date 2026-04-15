package server

import (
	"io/fs"
	"net/http"
)

// registerPWAHandlers mounts the manifest and service worker routes.
// Both files are served with no-cache headers so the browser always fetches
// the latest version on reload.
func registerPWAHandlers(mux interface{ Handle(pattern string, handler http.Handler) }, root fs.FS) {
	noCache := func(path, contentType string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			http.ServeFileFS(w, r, root, path)
		})
	}
	mux.Handle("GET /manifest.webmanifest", noCache("manifest.webmanifest", "application/manifest+json"))
	mux.Handle("GET /sw.js", noCache("sw.js", "application/javascript"))
}
