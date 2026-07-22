package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/taliove2009/hubscope/internal/server"
)

// plainGet issues a GET without any session cookie.
func plainGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// TestReadAuthTiers verifies the two-tier read model: status-board reads
// (overview, endpoint detail/series/probes) stay public while every other
// GET requires a session.
func TestReadAuthTiers(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	publicPaths := []string{
		"/api/overview",
		"/api/endpoints/1",
		"/api/endpoints/1/series",
		"/api/endpoints/1/probes",
	}
	for _, p := range publicPaths {
		resp := plainGet(t, ts.URL+p)
		resp.Body.Close()
		// 200 or 404 (no such endpoint yet) — but never 401.
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("public read %s: expected no 401, got 401", p)
		}
	}

	protectedPaths := []string{
		"/api/hubs",
		"/api/models",
		"/api/classification-rules",
		"/api/suites",
		"/api/evals",
		"/api/campaigns",
		"/api/settings",
		"/api/alerts",
		"/api/audit-logs",
		"/api/audit-logs/actions",
		"/api/tasks",
	}
	for _, p := range protectedPaths {
		resp := plainGet(t, ts.URL+p)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("protected read %s: expected 401, got %d", p, resp.StatusCode)
		}
		// With a session the same paths open up.
		resp = doGet(t, ts.URL+p)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("authed read %s: expected 200, got %d", p, resp.StatusCode)
		}
	}

	// Lookalike paths must never leak through the public-read pattern: they
	// get a 401 (protected) or a 404 (no route), never a 200.
	lookalikes := []string{
		"/api/overview/",
		"/api/overviewx",
		"/api/endpoints/abc",
		"/api/endpoints/1/foo",
		"/api/ENDPOINTS/1",
		"/api/endpoints/1/probesx",
	}
	for _, p := range lookalikes {
		resp := plainGet(t, ts.URL+p)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("lookalike %s must not be served publicly, got 200", p)
		}
	}
}

// loginAttempt posts one login with the wrong password and returns the status.
func loginAttempt(t *testing.T, baseURL string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest("POST", baseURL+"/api/auth/login",
		bytes.NewBufferString(`{"password":"wrong"}`))
	if err != nil {
		t.Fatalf("build login: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// TestRateLimitLogin verifies the strict per-IP limit on the login endpoint.
func TestRateLimitLogin(t *testing.T) {
	db := openTempDB(t)
	ts := httptest.NewServer(server.New(db, testAdminPassword,
		server.WithRateLimits(server.RateLimits{
			Login: server.RateTier{PerMinute: 2, Burst: 2},
		}),
	))
	t.Cleanup(ts.Close)

	if got := loginAttempt(t, ts.URL, nil); got != http.StatusUnauthorized {
		t.Fatalf("attempt 1: expected 401, got %d", got)
	}
	if got := loginAttempt(t, ts.URL, nil); got != http.StatusUnauthorized {
		t.Fatalf("attempt 2: expected 401, got %d", got)
	}
	if got := loginAttempt(t, ts.URL, nil); got != http.StatusTooManyRequests {
		t.Fatalf("attempt 3: expected 429, got %d", got)
	}
}

// TestRateLimitTrustProxy verifies X-Forwarded-For is honored only when the
// server is told to trust the proxy: with trust on, two forged IPs get
// independent budgets; with trust off, they share one.
func TestRateLimitTrustProxy(t *testing.T) {
	t.Run("trust_on", func(t *testing.T) {
		db := openTempDB(t)
		ts := httptest.NewServer(server.New(db, testAdminPassword,
			server.WithTrustProxy(true),
			server.WithRateLimits(server.RateLimits{
				Login: server.RateTier{PerMinute: 1, Burst: 1},
			}),
		))
		t.Cleanup(ts.Close)

		if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "1.1.1.1"}); got == http.StatusTooManyRequests {
			t.Fatal("first attempt from 1.1.1.1 should not be limited")
		}
		if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "2.2.2.2"}); got == http.StatusTooManyRequests {
			t.Error("2.2.2.2 should get its own budget when the proxy is trusted")
		}
		if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "1.1.1.1"}); got != http.StatusTooManyRequests {
			t.Errorf("second attempt from 1.1.1.1: expected 429, got %d", got)
		}
	})

	t.Run("trust_off", func(t *testing.T) {
		db := openTempDB(t)
		ts := httptest.NewServer(server.New(db, testAdminPassword,
			server.WithRateLimits(server.RateLimits{
				Login: server.RateTier{PerMinute: 1, Burst: 1},
			}),
		))
		t.Cleanup(ts.Close)

		if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "1.1.1.1"}); got == http.StatusTooManyRequests {
			t.Fatal("first attempt should not be limited")
		}
		// A forged XFF does not buy a fresh budget when the proxy is not trusted.
		if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "2.2.2.2"}); got != http.StatusTooManyRequests {
			t.Errorf("forged XFF should not bypass the limit: expected 429, got %d", got)
		}
	})
}

// TestRateLimitEntryCap verifies the per-tier bucket map is hard-capped: an
// IP spray (forged XFF with a trusted proxy) cannot grow it without bound —
// unknown IPs fail closed once the cap is reached.
func TestRateLimitEntryCap(t *testing.T) {
	db := openTempDB(t)
	ts := httptest.NewServer(server.New(db, testAdminPassword,
		server.WithTrustProxy(true),
		server.WithRateLimits(server.RateLimits{
			Login:             server.RateTier{PerMinute: 1000, Burst: 1000},
			MaxEntriesPerTier: 3,
		}),
	))
	t.Cleanup(ts.Close)

	// Three distinct IPs fill the table (each well within its own budget).
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": ip}); got == http.StatusTooManyRequests {
			t.Fatalf("%s: filling the table should not be limited, got 429", ip)
		}
	}
	// A fourth, never-seen IP fails closed even though its budget is fresh.
	if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "10.0.0.4"}); got != http.StatusTooManyRequests {
		t.Errorf("unknown IP over the cap: expected 429, got %d", got)
	}
	// A known IP still works.
	if got := loginAttempt(t, ts.URL, map[string]string{"X-Forwarded-For": "10.0.0.1"}); got == http.StatusTooManyRequests {
		t.Error("known IP must keep working while the table is full")
	}
}

// TestRateLimitReads verifies the public-read tier.
func TestRateLimitReads(t *testing.T) {
	db := openTempDB(t)
	ts := httptest.NewServer(server.New(db, testAdminPassword,
		server.WithRateLimits(server.RateLimits{
			Read: server.RateTier{PerMinute: 2, Burst: 2},
		}),
	))
	t.Cleanup(ts.Close)

	for i := 1; i <= 2; i++ {
		resp := plainGet(t, ts.URL+"/api/overview")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("read %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
	resp := plainGet(t, ts.URL+"/api/overview")
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("read 3: expected 429, got %d", resp.StatusCode)
	}
}

// TestRateLimitWrites verifies the write tier covers authenticated mutations.
func TestRateLimitWrites(t *testing.T) {
	db := openTempDB(t)
	ts := httptest.NewServer(server.New(db, testAdminPassword,
		server.WithRateLimits(server.RateLimits{
			Write: server.RateTier{PerMinute: 2, Burst: 2},
		}),
	))
	t.Cleanup(ts.Close)

	stub := newStubHubServer()
	defer stub.Close()

	createHubViaAPI(t, ts.URL, stub.URL)                // write 1
	resp := doPost(t, ts.URL+"/api/discovery/run", nil) // write 2
	resp.Body.Close()
	resp = doPost(t, ts.URL+"/api/discovery/run", nil) // write 3 → limited
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("write 3: expected 429, got %d", resp.StatusCode)
	}
}

// TestSecurityHeaders verifies the hardening headers on API responses.
func TestSecurityHeaders(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	resp := plainGet(t, ts.URL+"/api/overview")
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: expected nosniff, got %q", got)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("CSP should restrict framing, got %q", csp)
	} else if !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP should keep scripts self-only, got %q", csp)
	}
	if got := resp.Header.Get("Referrer-Policy"); got == "" {
		t.Error("Referrer-Policy should be set")
	}
}

// TestBodySizeLimit verifies oversized JSON bodies are rejected.
func TestBodySizeLimit(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	big := `{"name":"` + strings.Repeat("x", 2<<20) + `","base_url":"http://h","token":"t"}`
	req, err := http.NewRequest("POST", ts.URL+"/api/hubs", strings.NewReader(big))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := authedClient(t, ts.URL).Do(req)
	if err != nil {
		t.Fatalf("post big body: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body: expected 400, got %d", resp.StatusCode)
	}
}
