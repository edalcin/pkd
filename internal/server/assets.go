package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web/dist
var webFS embed.FS

func webRoot() fs.FS {
	sub, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		panic("server: could not sub webFS: " + err.Error())
	}
	return sub
}

func staticFileServer() http.Handler {
	return http.FileServer(http.FS(webRoot()))
}
