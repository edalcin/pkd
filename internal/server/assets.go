package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web
var webFS embed.FS

// webRoot returns a stripped fs.FS rooted at web/ so handlers can serve
// files directly without the "web/" path prefix.
func webRoot() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("server: could not sub webFS: " + err.Error())
	}
	return sub
}

// staticFileServer returns an http.Handler that serves files from the embedded
// web filesystem. Used for CSS, JS, vendor, and icon assets.
func staticFileServer() http.Handler {
	return http.FileServer(http.FS(webRoot()))
}
