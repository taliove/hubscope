// Package web exposes the embedded frontend build assets to the Go server.
package web

import "embed"

// DistFS holds the built frontend assets. The all: prefix ensures files whose
// names begin with '_' or '.' are also embedded. The Makefile's ensure-dist
// target guarantees dist/ exists (with a stub index.html) before any build.
//
//go:embed all:dist
var DistFS embed.FS
