package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// fallbackHTML is served when no built index.html is embedded yet.
const fallbackHTML = "hubscope: frontend not built yet"

// SetStaticFS registers the frontend asset filesystem (rooted at the dist
// directory) used to serve the SPA. Passing nil serves only the fallback text.
func (s *Server) SetStaticFS(fsys fs.FS) {
	s.staticFS = fsys
}

// serveSPA serves embedded frontend assets, falling back to index.html for
// client-side routes. Unmatched /api paths return a JSON 404 instead.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if s.staticFS == nil {
		serveFallback(w)
		return
	}

	// Try to serve the requested asset directly.
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" {
		if f, err := s.staticFS.Open(name); err == nil {
			if info, statErr := f.Stat(); statErr == nil && !info.IsDir() {
				_ = f.Close()
				http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r)
				return
			}
			_ = f.Close()
		}
	}

	// Fall back to index.html for SPA client-side routing.
	serveIndex(w, s.staticFS)
}

// serveIndex writes dist/index.html, or the fallback text if it is absent.
func serveIndex(w http.ResponseWriter, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		serveFallback(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// serveFallback writes a single-line placeholder response.
func serveFallback(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(fallbackHTML))
}
