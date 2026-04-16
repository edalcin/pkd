package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web
var webFS embed.FS

// webRoot returns a stripped fs.FS for serving static files.
// Prefers the Svelte build output at web/dist/ if it contains index.html;
// falls back to the legacy web/ tree (used only during development before
// the frontend has been built for the first time).
func webRoot() fs.FS {
	// Try Svelte build output first
	if _, err := webFS.Open("web/dist/index.html"); err == nil {
		if sub, err := fs.Sub(webFS, "web/dist"); err == nil {
			return sub
		}
	}
	// Fallback: serve the placeholder or legacy vanilla JS
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("server: could not sub webFS: " + err.Error())
	}
	return sub
}

// staticFileServer returns an http.Handler that serves static files from the
// embedded web filesystem.
func staticFileServer() http.Handler {
	return http.FileServer(http.FS(webRoot()))
}
