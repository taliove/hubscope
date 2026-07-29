package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// fallbackHTML is served when no built index.html is embedded yet.
const fallbackHTML = "hubscope: frontend not built yet"

// Cache-Control tiers for the embedded frontend (spec 0015 decision 2).
// embed.FS files carry a zero ModTime, so http.FileServer cannot emit
// Last-Modified and browsers re-download the ~1MB bundle on every visit.
// Content-hashed Vite artifacts are safe to cache forever; the HTML shell
// must stay no-cache so a release takes effect immediately (new HTML
// references new hashes, which cascades to fresh asset downloads).
const (
	cacheImmutable = "public, max-age=31536000, immutable" // /assets/* hashed build artifacts
	cacheShort     = "public, max-age=3600"                // root-level static files (favicon etc.)
	cacheNoCache   = "no-cache"                            // index.html and SPA route fallbacks
)

// SetStaticFS registers the frontend asset filesystem (rooted at the dist
// directory) used to serve the SPA. Passing nil serves only the fallback text.
func (s *Server) SetStaticFS(fsys fs.FS) {
	s.staticFS = fsys
}

// cacheControlFor classifies an embedded asset path into a cache tier.
// Vite emits all content-hashed files under assets/; everything else at the
// dist root is unhashed and gets only a short lifetime.
func cacheControlFor(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return cacheImmutable
	}
	if name == "index.html" {
		return cacheNoCache
	}
	return cacheShort
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
				w.Header().Set("Cache-Control", cacheControlFor(name))
				http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r)
				return
			}
			_ = f.Close()
		}
	}

	// Fall back to index.html for SPA client-side routing. The shell is
	// no-cache: a new release must be picked up immediately.
	w.Header().Set("Cache-Control", cacheNoCache)
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
