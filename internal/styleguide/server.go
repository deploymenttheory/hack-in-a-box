// Package styleguide serves the synthetic, static component guide independently
// of application sessions, APIs, PostgreSQL and identity providers.
package styleguide

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
)

type Server struct{ root *os.Root }

func Open(directory string) (*Server, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open static style guide: %w", err)
	}
	return &Server{root: root}, nil
}

func (s *Server) Close() error { return s.root.Close() }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().
		Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
		if r.URL.Path == "/readyz" {
			file, err := s.root.Open("index.html")
			if err != nil {
				http.Error(w, "Guide unavailable", http.StatusServiceUnavailable)
				return
			}
			info, err := file.Stat()
			_ = file.Close()
			if err != nil || !info.Mode().IsRegular() {
				http.Error(w, "Guide unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
		}
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "." || name == "" || name == "index.html" {
		name = "index.html"
	} else if !strings.HasPrefix(name, "assets/") {
		http.NotFound(w, r)
		return
	}
	file, err := s.root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	if name != "index.html" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}
