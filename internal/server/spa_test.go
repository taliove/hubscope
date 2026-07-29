package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/taliove/hubscope/internal/server"
)

// newStaticTestServer starts the API server with a fake embedded dist
// filesystem, mirroring the shape Vite produces (hashed assets/, root-level
// static files, index.html).
func newStaticTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := server.New(openTempDB(t),
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
	)
	srv.SetStaticFS(fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<html>spa</html>")},
		"assets/index-abc123.js": &fstest.MapFile{Data: []byte("console.log(1)")},
		"favicon.svg":            &fstest.MapFile{Data: []byte("<svg/>")},
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts
}

// getCacheControl issues a GET and returns the Cache-Control header.
func getCacheControl(t *testing.T, baseURL, path string) string {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", path, resp.StatusCode)
	}
	return resp.Header.Get("Cache-Control")
}

// TestStaticHashedAssetsImmutable verifies content-hashed build artifacts
// under /assets/ are cached immutably for a year (spec 0015 decision 2).
func TestStaticHashedAssetsImmutable(t *testing.T) {
	ts := newStaticTestServer(t)
	got := getCacheControl(t, ts.URL, "/assets/index-abc123.js")
	want := "public, max-age=31536000, immutable"
	if got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}

// TestSPAFallbackNoCache verifies the SPA shell (index.html and client-side
// route fallbacks) is served no-cache so a release takes effect immediately.
func TestSPAFallbackNoCache(t *testing.T) {
	ts := newStaticTestServer(t)
	for _, path := range []string{"/", "/index.html", "/endpoints/7", "/board"} {
		got := getCacheControl(t, ts.URL, path)
		if got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want %q", path, got, "no-cache")
		}
	}
}

// TestStaticRootFilesShortCache verifies root-level static files that are not
// content-hashed (favicon etc.) get a short cache lifetime.
func TestStaticRootFilesShortCache(t *testing.T) {
	ts := newStaticTestServer(t)
	got := getCacheControl(t, ts.URL, "/favicon.svg")
	want := "public, max-age=3600"
	if got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}
