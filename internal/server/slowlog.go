package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// defaultSlowRequestThreshold is the /api latency budget above which a
// request is logged as slow (spec 0015 decision 10). It gives the overview
// optimization work a before/after measurement baseline in production logs.
const defaultSlowRequestThreshold = 200 * time.Millisecond

// WithSlowRequestLog overrides the slow-request threshold and logger. Tests
// inject a nanosecond threshold with a buffered logger so the warn line is
// observable without sleeping (the WithRateLimits precedent). A nil logger
// keeps the default (slog.Default()).
func WithSlowRequestLog(threshold time.Duration, logger *slog.Logger) Option {
	return func(s *Server) {
		s.slowLogThreshold = threshold
		if logger != nil {
			s.slowLogger = logger
		}
	}
}

// slowRequestLogger warns on /api requests slower than the configured
// threshold. The line carries method + route pattern + duration — never the
// body, and never the raw path: parameterized segments may carry credentials
// (ADR 0006 share tokens in /api/shared-reports/{token}), and W6 keeps
// credentials out of logs. Cost on the hot path is one time.Now pair per
// request; nothing else is allocated. Mounted only on the /api route group,
// so /healthz and static assets stay silent.
func (s *Server) slowRequestLogger(next http.Handler) http.Handler {
	threshold := s.slowLogThreshold
	if threshold <= 0 {
		threshold = defaultSlowRequestThreshold
	}
	logger := s.slowLogger
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if d := time.Since(start); d > threshold {
			logger.Warn("slow request",
				"method", r.Method,
				"route", logRoute(r),
				"duration_ms", d.Milliseconds(),
			)
		}
	})
}

// logRoute returns the matched chi route pattern (e.g.
// "/api/shared-reports/{token}") rather than the request path, so credential-
// bearing segments are never logged (W6). chi populates the pattern before
// the middleware chain runs; unmatched paths (404s) collapse to "unmatched".
func logRoute(r *http.Request) string {
	if p := chi.RouteContext(r.Context()).RoutePattern(); p != "" {
		return p
	}
	return "unmatched"
}
