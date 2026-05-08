package server

import (
	"net/http"
	"os"
)

func staticHandler() http.Handler {
	for _, dir := range []string{"internal/server/dist", "web/dist"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return http.FileServer(http.Dir(dir))
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "OpenScholar Web GUI is not bundled in this build. Build web assets with: cd web && npm run build", http.StatusNotImplemented)
	})
}
