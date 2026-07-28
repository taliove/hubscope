package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// These tests cover spec 0012 (adaptive captcha login, ticket 88) at the W1
// seam: they only observe HTTP behavior (status codes, the captcha_required
// marker, error messages, response-time magnitude) and never assert internal
// counter or store state. The captcha trigger policy is injected via
// WithCaptchaPolicy and the answer store via WithCaptchaStore — tests seed
// known answers through the store's Seed seam (the WithLoginDelay /
// WithRateLimits precedent). Per-IP rate limits and the progressive delay
// are disabled except where a test targets them.

// captchaTestServer starts the API server with rate limits and the login
// delay disabled and the given captcha trigger policy + answer store.
// Extra options override the disabled defaults (later options win).
func captchaTestServer(t *testing.T, db *store.DB, policy server.CaptchaPolicy, st *server.CaptchaStore, extra ...server.Option) *httptest.Server {
	t.Helper()
	opts := append([]server.Option{
		server.WithRateLimits(server.RateLimits{}),
		server.WithLoginDelay(server.LoginDelayPolicy{}),
		server.WithCaptchaPolicy(policy),
		server.WithCaptchaStore(st),
	}, extra...)
	ts := httptest.NewServer(server.New(db, opts...))
	t.Cleanup(ts.Close)
	return ts
}

// captchaLogin posts one login attempt (optionally carrying captcha fields
// and a spoofed X-Forwarded-For source) and reports the status, the error
// message (empty on success), the captcha_required marker, and the
// wall-clock duration.
func captchaLogin(t *testing.T, baseURL, username, password, captchaID, captchaAnswer, ip string) (int, string, bool, time.Duration) {
	t.Helper()
	body := `{"username":` + strconv.Quote(username) + `,"password":` + strconv.Quote(password)
	if captchaID != "" || captchaAnswer != "" {
		body += `,"captcha_id":` + strconv.Quote(captchaID) + `,"captcha_answer":` + strconv.Quote(captchaAnswer)
	}
	body += `}`
	req, err := http.NewRequest("POST", baseURL+"/api/auth/login", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login attempt: %v", err)
	}
	elapsed := time.Since(start)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return resp.StatusCode, "", false, elapsed
	}
	var env struct {
		Error struct {
			Message         string `json:"message"`
			CaptchaRequired bool   `json:"captcha_required"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return resp.StatusCode, env.Error.Message, env.Error.CaptchaRequired, elapsed
}

// issueCaptcha GETs one captcha and reports the status plus the parsed data
// payload (empty on error).
func issueCaptcha(t *testing.T, baseURL string) (int, string, string) {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/auth/captcha")
	if err != nil {
		t.Fatalf("issue captcha: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, "", ""
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode captcha envelope: %v", err)
	}
	var data struct {
		CaptchaID string `json:"captcha_id"`
		Image     string `json:"image"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal captcha data: %v", err)
	}
	return resp.StatusCode, data.CaptchaID, data.Image
}

// requireCaptcha asserts one attempt is answered 401 with the
// captcha_required marker and the expected frozen message.
func requireCaptcha(t *testing.T, status int, msg string, required bool, wantMsg string) {
	t.Helper()
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
	if !required {
		t.Fatalf("expected captcha_required marker, message %q", msg)
	}
	if msg != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, msg)
	}
}

// requirePlainFailure asserts one attempt is answered 401 with the uniform
// invalid-credentials message and NO captcha_required marker.
func requirePlainFailure(t *testing.T, status int, msg string, required bool) {
	t.Helper()
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
	if required {
		t.Fatalf("expected no captcha_required marker, message %q", msg)
	}
	if msg != "invalid credentials" {
		t.Fatalf("expected uniform error, got %q", msg)
	}
}

// TestCaptchaTriggerRequiresAfterTwoFailures covers spec test decision 1:
// after 2 failures for one username, the 3rd attempt without a captcha is
// answered 401 + captcha_required; a wrong captcha keeps the marker; a
// correct captcha plus the correct password passes.
func TestCaptchaTriggerRequiresAfterTwoFailures(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st)

	for i := 1; i <= 2; i++ {
		status, msg, required, _ := captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
		if status != http.StatusUnauthorized || required {
			t.Fatalf("attempt %d within threshold: expected plain 401, got %d (required=%v, %q)", i, status, required, msg)
		}
	}

	// 3rd attempt without a captcha: required.
	status, msg, required, _ := captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	requireCaptcha(t, status, msg, required, "请完成验证码")

	// A wrong captcha answer keeps the requirement and the marker.
	st.Seed("seed-wrong", "123456")
	status, msg, required, _ = captchaLogin(t, ts.URL, "admin", "wrong-password", "seed-wrong", "000000", "")
	requireCaptcha(t, status, msg, required, "验证码错误或已过期")

	// A correct captcha plus the correct password passes.
	st.Seed("seed-ok", "123456")
	status, _, _, _ = captchaLogin(t, ts.URL, "admin", testAdminPassword, "seed-ok", "123456", "")
	if status != http.StatusOK {
		t.Fatalf("correct captcha + correct password: expected 200, got %d", status)
	}
}

// TestCaptchaTriggerDualDimensionIndependent covers spec test decision 2:
// the IP and username dimensions trigger independently — switching username
// does not clear an IP's requirement, switching IP does not clear a
// username's requirement, and an untouched pair stays captcha-free.
func TestCaptchaTriggerDualDimensionIndependent(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st,
		server.WithTrustProxy(true))

	// Two failures for admin from 10.0.0.1 arm both dimensions.
	for i := 1; i <= 2; i++ {
		status, _, _, _ := captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "10.0.0.1")
		if status != http.StatusUnauthorized {
			t.Fatalf("seeding attempt %d: expected 401, got %d", i, status)
		}
	}

	// A different username from the marked IP is still required (IP dim).
	status, msg, required, _ := captchaLogin(t, ts.URL, "ghost", "wrong-password", "", "", "10.0.0.1")
	requireCaptcha(t, status, msg, required, "请完成验证码")

	// The marked username from a fresh IP is still required (username dim).
	status, msg, required, _ = captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "10.0.0.2")
	requireCaptcha(t, status, msg, required, "请完成验证码")

	// An untouched (username, IP) pair is not required.
	status, msg, required, _ = captchaLogin(t, ts.URL, "ghost", "wrong-password", "", "", "10.0.0.2")
	requirePlainFailure(t, status, msg, required)
}

// TestCaptchaOneTimeUse covers spec test decision 3: any verification
// attempt destroys the captcha — even the correct answer fails on reuse.
func TestCaptchaOneTimeUse(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st)

	captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")

	// First attempt with the wrong answer consumes the captcha.
	st.Seed("seed-once", "123456")
	status, msg, required, _ := captchaLogin(t, ts.URL, "admin", "wrong-password", "seed-once", "000000", "")
	requireCaptcha(t, status, msg, required, "验证码错误或已过期")

	// Reuse with the CORRECT answer (and the correct password) must still
	// fail — the captcha was destroyed by the first attempt. If it had
	// survived, this attempt would pass with 200.
	status, msg, required, _ = captchaLogin(t, ts.URL, "admin", testAdminPassword, "seed-once", "123456", "")
	requireCaptcha(t, status, msg, required, "验证码错误或已过期")
}

// TestCaptchaTTLExpiry covers spec test decision 4: an expired captcha
// answers "验证码错误或已过期" even with the correct answer.
func TestCaptchaTTLExpiry(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: 50 * time.Millisecond})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st)

	captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")

	st.Seed("seed-ttl", "123456")
	time.Sleep(100 * time.Millisecond)
	status, msg, required, _ := captchaLogin(t, ts.URL, "admin", testAdminPassword, "seed-ttl", "123456", "")
	requireCaptcha(t, status, msg, required, "验证码错误或已过期")
}

// TestCaptchaFailureDoesNotAdvanceDelay covers spec test decision 5: a
// captcha-stage failure neither earns a progressive-delay sleep itself nor
// counts toward the per-account delay threshold — spec 0011's semantics are
// neither bypassed nor doubled by the captcha path.
func TestCaptchaFailureDoesNotAdvanceDelay(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st,
		server.WithLoginDelay(server.LoginDelayPolicy{
			Threshold: 3,
			Window:    time.Minute,
			Backoff:   []time.Duration{100 * time.Millisecond, 600 * time.Millisecond},
		}))

	// Two password failures arm the captcha trigger (delay count: 2).
	for i := 1; i <= 2; i++ {
		_, _, _, elapsed := captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
		if elapsed >= 50*time.Millisecond {
			t.Fatalf("password failure %d within delay threshold must be fast, took %v", i, elapsed)
		}
	}

	// Five captcha-stage failures: no captcha, then four wrong answers.
	// None may sleep (risk 5: no 2-8s waits on the captcha path).
	_, _, _, elapsed := captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	if elapsed >= 50*time.Millisecond {
		t.Fatalf("missing-captcha rejection must not be delayed, took %v", elapsed)
	}
	for i := 1; i <= 4; i++ {
		id := "seed-delay-wrong-" + strconv.Itoa(i)
		st.Seed(id, "123456")
		_, _, _, elapsed = captchaLogin(t, ts.URL, "admin", "wrong-password", id, "000000", "")
		if elapsed >= 50*time.Millisecond {
			t.Fatalf("wrong-captcha rejection %d must not be delayed, took %v", i, elapsed)
		}
	}

	// Password failure #3 (via a correct captcha): still within the delay
	// threshold — fast.
	st.Seed("seed-delay-ok-1", "123456")
	status, msg, required, elapsed := captchaLogin(t, ts.URL, "admin", "wrong-password", "seed-delay-ok-1", "123456", "")
	requirePlainFailure(t, status, msg, required)
	if elapsed >= 50*time.Millisecond {
		t.Fatalf("password failure 3 within delay threshold must be fast, took %v", elapsed)
	}

	// Password failure #4: over the delay threshold → first band ~100ms.
	// Had the five captcha failures counted toward the delay, the count
	// would be deep in the second band (~600ms). The 500ms upper bound
	// leaves generous CI slack while still separating the two bands.
	st.Seed("seed-delay-ok-2", "123456")
	status, msg, required, elapsed = captchaLogin(t, ts.URL, "admin", "wrong-password", "seed-delay-ok-2", "123456", "")
	requirePlainFailure(t, status, msg, required)
	if elapsed < 90*time.Millisecond {
		t.Fatalf("password failure 4 past delay threshold: expected ~100ms penalty, took %v", elapsed)
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("captcha failures must not advance the delay band (would be ~600ms), took %v", elapsed)
	}
}

// TestCaptchaSuccessResetsBothDimensions covers spec test decision 6: a
// successful login clears the captcha requirement for both the username and
// the source IP, until failures accumulate again.
func TestCaptchaSuccessResetsBothDimensions(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st)

	captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	status, msg, required, _ := captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	requireCaptcha(t, status, msg, required, "请完成验证码")

	st.Seed("seed-reset", "123456")
	status, _, _, _ = captchaLogin(t, ts.URL, "admin", testAdminPassword, "seed-reset", "123456", "")
	if status != http.StatusOK {
		t.Fatalf("correct captcha + correct password: expected 200, got %d", status)
	}

	// The username dimension is clear: one new failure stays captcha-free.
	status, msg, required, _ = captchaLogin(t, ts.URL, "admin", "wrong-password", "", "", "")
	requirePlainFailure(t, status, msg, required)

	// The IP dimension is clear too: a different username from the same
	// client IP is not required.
	status, msg, required, _ = captchaLogin(t, ts.URL, "ghost", "wrong-password", "", "", "")
	requirePlainFailure(t, status, msg, required)
}

// TestCaptchaIssueRateLimited covers spec test decision 7: the issue
// endpoint has its own per-IP tier — over budget answers 429.
func TestCaptchaIssueRateLimited(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{}, st,
		server.WithRateLimits(server.RateLimits{
			Captcha: server.RateTier{PerMinute: 2, Burst: 2},
		}))

	for i := 1; i <= 2; i++ {
		status, id, image := issueCaptcha(t, ts.URL)
		if status != http.StatusOK {
			t.Fatalf("issue %d: expected 200, got %d", i, status)
		}
		if id == "" {
			t.Fatalf("issue %d: captcha_id must not be empty", i)
		}
		if !strings.HasPrefix(image, "data:image/png;base64,") {
			t.Fatalf("issue %d: image must be a PNG data URI, got prefix %q", i, image[:min(len(image), 32)])
		}
	}
	status, _, _ := issueCaptcha(t, ts.URL)
	if status != http.StatusTooManyRequests {
		t.Fatalf("issue 3 over budget: expected 429, got %d", status)
	}
}

// TestCaptchaStoreFullFailsClosed verifies a full answer store rejects new
// issues with 503 rather than silently letting clients skip the captcha
// (fail-closed, spec decision 2).
func TestCaptchaStoreFullFailsClosed(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute, MaxEntries: 2})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{}, st)

	for i := 1; i <= 2; i++ {
		status, _, _ := issueCaptcha(t, ts.URL)
		if status != http.StatusOK {
			t.Fatalf("issue %d: expected 200, got %d", i, status)
		}
	}
	status, _, _ := issueCaptcha(t, ts.URL)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("issue over the store cap: expected 503, got %d", status)
	}
	resp, err := http.Get(ts.URL + "/api/auth/captcha")
	if err != nil {
		t.Fatalf("issue captcha: %v", err)
	}
	if msg := readErrorMessage(t, resp); msg != "验证码服务暂不可用,请稍后重试" {
		t.Fatalf("store-full message: expected frozen wording, got %q", msg)
	}
}

// TestCaptchaFieldsIgnoredWhenNotRequired covers spec decision 3: captcha
// fields on a login that is not required to have one are ignored — no
// error, no verification, old clients stay compatible.
func TestCaptchaFieldsIgnoredWhenNotRequired(t *testing.T) {
	db := openTempDB(t)
	st := server.NewCaptchaStore(server.CaptchaStorePolicy{TTL: time.Minute})
	ts := captchaTestServer(t, db, server.CaptchaPolicy{Threshold: 2, Window: time.Minute}, st)

	// Correct password + bogus captcha fields: passes (fields ignored).
	status, _, _, _ := captchaLogin(t, ts.URL, "admin", testAdminPassword, "nonexistent-id", "000000", "")
	if status != http.StatusOK {
		t.Fatalf("unrequired login with captcha fields: expected 200, got %d", status)
	}

	// Wrong password + bogus captcha fields: the uniform failure, no marker.
	status, msg, required, _ := captchaLogin(t, ts.URL, "admin", "wrong-password", "nonexistent-id", "000000", "")
	requirePlainFailure(t, status, msg, required)
}
