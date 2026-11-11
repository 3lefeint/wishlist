package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spa serves the built frontend. Unknown paths answer index.html so the client
// router can handle them; unknown API paths stay 404.
func spa(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if st, err := fs.Stat(files, name); err != nil || st.IsDir() {
			name = "index.html"
			r.URL.Path = "/"
		}
		switch {
		case strings.HasPrefix(name, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
