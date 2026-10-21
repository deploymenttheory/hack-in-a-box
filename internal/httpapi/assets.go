package httpapi

import (
	"net/http"
	"os"
	"path"
	"strings"
)

func assets(directory string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			notFound(w, r)
			return
		}
		for _, prefix := range []string{"/api", "/auth", "/ops"} {
			if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
				notFound(w, r)
				return
			}
		}
		root, err := os.OpenRoot(directory)
		if err != nil {
			notFound(w, r)
			return
		}
		defer func() { _ = root.Close() }()
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if strings.HasPrefix(name, "assets/") {
			file, err := root.Open(name)
			if err != nil {
				notFound(w, r)
				return
			}
			defer func() { _ = file.Close() }()
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				notFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(w, r, name, info.ModTime(), file)
			return
		}
		if path.Ext(name) != "" {
			notFound(w, r)
			return
		}
		file, err := root.Open("index.html")
		if err != nil {
			notFound(w, r)
			return
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			notFound(w, r)
			return
		}
		w.Header().
			Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "same-origin")
		http.ServeContent(w, r, "index.html", info.ModTime(), file)
	}
}
