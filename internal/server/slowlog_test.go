package server_test

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
)

// syncBuffer is a goroutine-safe log sink: the httptest server logs from its
// handler goroutine while the test goroutine reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForLog polls the sink until want appears or the deadline passes. The
// warn line is written after the response body, so the client may observe the
// response before the log lands.
func waitForLog(sink *syncBuffer, want string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(sink.String(), want) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestSlowRequestLogEmitted verifies that an /api request exceeding the
// (injected, nanosecond) threshold produces one warn log line carrying
// method, route pattern, and duration — never the body, never the raw path.
func TestSlowRequestLogEmitted(t *testing.T) {
	db := openTempDB(t)
	sink := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(sink, nil))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSlowRequestLog(time.Nanosecond, logger),
	))
	t.Cleanup(ts.Close)

	resp := plainGet(t, ts.URL+"/api/overview")
	resp.Body.Close()

	if !waitForLog(sink, "slow request") {
		t.Fatalf("expected slow-request warn log, got: %q", sink.String())
	}
	line := sink.String()
	for _, want := range []string{"level=WARN", "method=GET", `route=/api/overview`, "duration_ms="} {
		if !strings.Contains(line, want) {
			t.Errorf("slow-request log missing %q, got: %q", want, line)
		}
	}
}

// TestSlowRequestLogRedactsShareToken verifies W6: a slow request to a
// credential-bearing route (/api/shared-reports/{token}, ADR 0006) logs the
// route pattern — the token itself never lands in the log (W6 review finding:
// the middleware previously logged r.URL.Path verbatim).
func TestSlowRequestLogRedactsShareToken(t *testing.T) {
	db := openTempDB(t)
	sink := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(sink, nil))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSlowRequestLog(time.Nanosecond, logger),
	))
	t.Cleanup(ts.Close)

	resp := plainGet(t, ts.URL+"/api/shared-reports/SECRETTOKEN123")
	resp.Body.Close()

	if !waitForLog(sink, "slow request") {
		t.Fatalf("expected slow-request warn log, got: %q", sink.String())
	}
	line := sink.String()
	if strings.Contains(line, "SECRETTOKEN123") {
		t.Fatalf("share token leaked into slow-request log: %q", line)
	}
	if !strings.Contains(line, `route=/api/shared-reports/{token}`) {
		t.Errorf("expected route pattern in log, got: %q", line)
	}
}

// TestSlowRequestLogSilentUnderThreshold verifies that a request comfortably
// under the threshold produces no log line (threshold set to an hour — a
// request slower than that cannot complete within the test).
func TestSlowRequestLogSilentUnderThreshold(t *testing.T) {
	db := openTempDB(t)
	sink := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(sink, nil))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSlowRequestLog(time.Hour, logger),
	))
	t.Cleanup(ts.Close)

	resp := plainGet(t, ts.URL+"/api/overview")
	resp.Body.Close()

	if out := sink.String(); strings.Contains(out, "slow request") {
		t.Fatalf("expected no slow-request log under threshold, got: %q", out)
	}
}

// TestSlowRequestLogSkipsNonAPI verifies the middleware is scoped to /api:
// infrastructure endpoints (/healthz) never generate slow-request noise even
// past the threshold.
func TestSlowRequestLogSkipsNonAPI(t *testing.T) {
	db := openTempDB(t)
	sink := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(sink, nil))
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSlowRequestLog(time.Nanosecond, logger),
	))
	t.Cleanup(ts.Close)

	resp := plainGet(t, ts.URL+"/healthz")
	resp.Body.Close()

	if out := sink.String(); strings.Contains(out, "slow request") {
		t.Fatalf("expected no slow-request log for /healthz, got: %q", out)
	}
}
