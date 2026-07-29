package server_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// These tests cover spec 0011 decision 4 (instance-level brute-force login
// alert, ticket 86) at the W1 seam: they only observe HTTP behavior (status
// codes, response-time magnitude) and the Lark webhook side effect, never
// internal counter state. The policy is injected via WithLoginAlert at
// millisecond scale (the WithLoginDelay precedent), and both the per-IP
// login limiter and the per-account progressive delay are disabled — they
// are orthogonal mechanisms that would otherwise answer 429 or slow the
// suite before the alert threshold is reached.

// bruteTestPassword is the wrong password used by every attempt in this
// file; its distinctive value lets the content test prove the alert text
// never carries a password.
const bruteTestPassword = "s3cr3t-brute-password"

// loginAlertTestServer starts the API server with rate limits and the login
// delay disabled and the given millisecond-scale login-alert policy. The
// adaptive captcha trigger is disabled too (zero policy — the WithRateLimits
// precedent): from the 3rd failure on it would answer with the
// captcha_required wording instead of the uniform message this file asserts
// (ticket 88 decision: spec 0011 tests keep their behavior semantics, they
// just opt the new layer out).
func loginAlertTestServer(t *testing.T, db *store.DB, policy server.LoginAlertPolicy, extra ...server.Option) *httptest.Server {
	t.Helper()
	opts := append([]server.Option{
		server.WithRateLimits(server.RateLimits{}),
		server.WithLoginDelay(server.LoginDelayPolicy{}),
		server.WithCaptchaPolicy(server.CaptchaPolicy{}),
		server.WithLoginAlert(policy),
	}, extra...)
	ts := httptest.NewServer(server.New(db, opts...))
	t.Cleanup(ts.Close)
	return ts
}

// failedLogin posts one wrong-password attempt (optionally with a spoofed
// X-Forwarded-For source) and requires the uniform 401 answer.
func failedLogin(t *testing.T, baseURL, username, ip string) {
	t.Helper()
	req, err := http.NewRequest("POST", baseURL+"/api/auth/login",
		strings.NewReader(`{"username":`+strconv.Quote(username)+`,"password":`+strconv.Quote(bruteTestPassword)+`}`))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login attempt: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		resp.Body.Close()
		t.Fatalf("failed login for %q: expected 401, got %d", username, resp.StatusCode)
	}
	if msg := readErrorMessage(t, resp); msg != "invalid credentials" {
		t.Fatalf("failed login for %q: expected uniform error, got %q", username, msg)
	}
}

// waitForLarkMessages polls the stub until it holds at least n messages or
// the deadline passes. Alert sends are asynchronous by design (the login
// response path must not block), so the black-box observation is a bounded
// poll rather than a fixed sleep.
func waitForLarkMessages(t *testing.T, lark *stubLarkServer, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		msgs := lark.messages()
		if len(msgs) >= n {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d lark messages, got %d", n, len(msgs))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// configureWebhook points the server's alerting at the stub with the given
// enabled switch.
func configureWebhook(t *testing.T, ts *httptest.Server, lark *stubLarkServer, enabled bool) {
	t.Helper()
	resp := doPut(t, ts.URL+"/api/settings", map[string]interface{}{
		"lark_webhook_url": lark.URL,
		"alert_enabled":    enabled,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d", resp.StatusCode)
	}
}

// TestLoginBruteForceAlertThresholdFiresOnce verifies reaching the
// instance-level failure threshold fires exactly one Lark alert (spec test
// decision 4).
func TestLoginBruteForceAlertThresholdFiresOnce(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  time.Minute,
	})
	configureWebhook(t, ts, lark, true)

	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	msgs := waitForLarkMessages(t, lark, 1)
	if len(msgs) != 1 {
		t.Fatalf("crossing the threshold: expected exactly 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "3") {
		t.Errorf("alert should carry the failure count, got: %s", msgs[0])
	}
}

// TestLoginBruteForceAlertCooldownSuppresses verifies further failures
// inside the cooldown window do not re-send (spec test decision 4).
func TestLoginBruteForceAlertCooldownSuppresses(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  600 * time.Millisecond,
	})
	configureWebhook(t, ts, lark, true)

	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	waitForLarkMessages(t, lark, 1)

	// More failures inside the cooldown stay silent. The assert lands at
	// roughly half the cooldown, so CI jitter cannot cross the boundary.
	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	time.Sleep(300 * time.Millisecond)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("inside cooldown: expected still 1 message, got %d", got)
	}
}

// TestLoginBruteForceAlertFiresAgainAfterCooldown verifies a new burst may
// alert again once the cooldown has elapsed.
func TestLoginBruteForceAlertFiresAgainAfterCooldown(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  300 * time.Millisecond,
	})
	configureWebhook(t, ts, lark, true)

	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	waitForLarkMessages(t, lark, 1)

	time.Sleep(400 * time.Millisecond)
	failedLogin(t, ts.URL, "admin", "")
	msgs := waitForLarkMessages(t, lark, 2)
	if !strings.Contains(msgs[1], "4") {
		t.Errorf("second alert should carry the renewed window count (4), got: %s", msgs[1])
	}
}

// TestLoginBruteForceAlertMessageContent verifies the alert text carries the
// count, the window, and the top-3 usernames and source IPs — and never a
// password or the webhook address (W6).
func TestLoginBruteForceAlertMessageContent(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 5,
		Window:    time.Minute,
		Cooldown:  time.Minute,
	}, server.WithTrustProxy(true))
	configureWebhook(t, ts, lark, true)

	failedLogin(t, ts.URL, "admin", "10.0.0.1")
	failedLogin(t, ts.URL, "admin", "10.0.0.1")
	failedLogin(t, ts.URL, "root", "10.0.0.2")
	failedLogin(t, ts.URL, "root", "10.0.0.2")
	failedLogin(t, ts.URL, "ghost", "10.0.0.3")

	msgs := waitForLarkMessages(t, lark, 1)
	for _, want := range []string{"5", "admin", "root", "ghost", "10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("alert should contain %q, got: %s", want, msgs[0])
		}
	}
	if strings.Contains(msgs[0], bruteTestPassword) {
		t.Errorf("alert must never contain a password, got: %s", msgs[0])
	}
	if strings.Contains(msgs[0], lark.URL) {
		t.Errorf("alert must never contain the webhook address, got: %s", msgs[0])
	}

	// The brute-force alert goes out as a red-header interactive card
	// (ticket 101) with the count, window, and top lists as structured
	// fields.
	cards := lark.cards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if cards[0].Template != "red" {
		t.Errorf("brute-force card template: expected red, got %q", cards[0].Template)
	}
	if cards[0].Title != "登录爆破告警 · HubScope" {
		t.Errorf("brute-force card title: got %q", cards[0].Title)
	}
	if cards[0].Fields["失败次数"] != "5 次" {
		t.Errorf("brute-force card 失败次数 field: got %q", cards[0].Fields["失败次数"])
	}
	if !strings.Contains(cards[0].Fields["被尝试最多的用户名"], "admin") {
		t.Errorf("brute-force card 用户名 field should list admin, got %q", cards[0].Fields["被尝试最多的用户名"])
	}
	if !strings.Contains(cards[0].Fields["失败最多的来源 IP"], "10.0.0.1") {
		t.Errorf("brute-force card 来源 IP field should list 10.0.0.1, got %q", cards[0].Fields["失败最多的来源 IP"])
	}
}

// TestLoginBruteForceAlertSilentWithoutWebhook verifies an unconfigured
// webhook (or the disabled alert switch) produces no messages and no errors
// while failures cross the threshold (mirrors TestAlertSkippedWithoutWebhook).
func TestLoginBruteForceAlertSilentWithoutWebhook(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  300 * time.Millisecond,
	})

	// Webhook never configured: crossing the threshold sends nothing.
	for i := 0; i < 4; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	time.Sleep(200 * time.Millisecond)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("no webhook configured: expected no messages, got %d", got)
	}

	// Webhook configured but alerts disabled, cooldown already elapsed:
	// crossing the threshold again still sends nothing.
	configureWebhook(t, ts, lark, false)
	time.Sleep(350 * time.Millisecond)
	for i := 0; i < 2; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	time.Sleep(200 * time.Millisecond)
	if got := len(lark.messages()); got != 0 {
		t.Fatalf("alerts disabled: expected no messages, got %d", got)
	}
}

// TestLoginBruteForceAlertDoesNotBlockLogin verifies the webhook round trip
// never lands on the login response path: the attempt that crosses the
// threshold answers promptly even while the stub hangs (ticket risk 1).
func TestLoginBruteForceAlertDoesNotBlockLogin(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	lark.setDelay(500 * time.Millisecond)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  time.Minute,
	})
	configureWebhook(t, ts, lark, true)

	failedLogin(t, ts.URL, "admin", "")
	failedLogin(t, ts.URL, "admin", "")
	start := time.Now()
	failedLogin(t, ts.URL, "admin", "")
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Fatalf("threshold-crossing login must not wait for the alert send, took %v", elapsed)
	}

	// The send still completes in the background (and this drains the
	// in-flight goroutine before the stub closes).
	waitForLarkMessages(t, lark, 1)
}

// TestLoginBruteForceAlertSendFailureNotRetried verifies a failing webhook
// neither blocks nor gets retried on every subsequent failure — one attempt
// per cooldown, logged, done (mirrors TestAlertSendFailureRecorded).
func TestLoginBruteForceAlertSendFailureNotRetried(t *testing.T) {
	db := openTempDB(t)
	lark := newStubLarkServer(t)
	lark.setStatus(http.StatusInternalServerError)
	ts := loginAlertTestServer(t, db, server.LoginAlertPolicy{
		Threshold: 3,
		Window:    time.Minute,
		Cooldown:  time.Minute,
	})
	configureWebhook(t, ts, lark, true)

	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	// The stub records every attempt, including failed ones.
	waitForLarkMessages(t, lark, 1)

	for i := 0; i < 3; i++ {
		failedLogin(t, ts.URL, "admin", "")
	}
	time.Sleep(200 * time.Millisecond)
	if got := len(lark.messages()); got != 1 {
		t.Fatalf("failed send must not be retried inside the cooldown, got %d attempts", got)
	}
}
