package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var staticFS embed.FS

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "dist")
	if err != nil {
		// Fallback: serve the raw embedded FS
		return http.FileServerFS(staticFS)
	}
	return http.FileServerFS(sub)
}
