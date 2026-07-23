package server_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/taliove2009/hubscope/internal/store"
)

// testAdminPassword is the password for the test super_admin seeded into
// every test database; it is not a real credential.
const testAdminPassword = "test-admin-password"

// testSessionSecret is a fixed 32-byte signing key injected via
// WithSessionSecret so forged tokens can be reproduced without reading the
// database. It mirrors the format server.New would auto-generate (32 raw
// bytes) but is deterministic for tests.
var testSessionSecret = bytes.Repeat([]byte{0xAB}, 32)

// seedTestUser ensures the test super_admin (username "admin", password
// testAdminPassword) exists in the database. It is idempotent: if the user
// already exists (e.g., a closure re-opens the same DB file), it silently
// succeeds. bcrypt cost 4 keeps tests fast; the hash is self-contained so
// no WithBcryptCost option is needed on the server.
func seedTestUser(t *testing.T, db *store.DB) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testAdminPassword), 4)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	if _, err := db.CreateUser("admin", string(hash), nil, store.RoleSuperAdmin); err != nil {
		if err == store.ErrUsernameTaken {
			return // already seeded — idempotent
		}
		t.Fatalf("seed test user: %v", err)
	}
}

// testClients caches one logged-in HTTP client per test server origin, so
// shared request helpers transparently pass the auth middleware while test
// assertions stay unchanged.
var testClients sync.Map // origin -> *http.Client

// authedClient returns an HTTP client that has logged in against the server
// hosting rawURL, carrying the session cookie in its jar.
func authedClient(t *testing.T, rawURL string) *http.Client {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url %q: %v", rawURL, err)
	}
	origin := u.Scheme + "://" + u.Host
	if cached, ok := testClients.Load(origin); ok {
		return cached.(*http.Client)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp, err := client.Post(origin+"/api/auth/login", "application/json",
		bytes.NewBufferString(`{"username":"admin","password":`+strconv.Quote(testAdminPassword)+`}`))
	if err != nil {
		t.Fatalf("test login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test login: expected 200, got %d", resp.StatusCode)
	}

	actual, _ := testClients.LoadOrStore(origin, client)
	return actual.(*http.Client)
}

// forgeSessionToken reproduces the server-side signing scheme so tests can
// mint expired or tampered cookies without any server cooperation. The token
// format is "<userId>.<issuedUnix>.<hmacHex>".
func forgeSessionToken(t *testing.T, userID int64, issued time.Time, tamper bool) string {
	t.Helper()
	uid := strconv.FormatInt(userID, 10)
	stamp := strconv.FormatInt(issued.Unix(), 10)
	payload := uid + "." + stamp
	mac := hmac.New(sha256.New, testSessionSecret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	if tamper {
		replacement := "a"
		if sig[0] == 'a' {
			replacement = "b"
		}
		sig = replacement + sig[1:]
	}
	return payload + "." + sig
}

// postRaw issues an unauthenticated POST and returns the response.
func postRaw(t *testing.T, client *http.Client, rawURL string, body string) *http.Response {
	t.Helper()
	resp, err := client.Post(rawURL, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s: %v", rawURL, err)
	}
	return resp
}

// createHub issues POST /api/hubs with the given client and cookie header.
func createHub(t *testing.T, client *http.Client, baseURL, cookie string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", baseURL+"/api/hubs",
		bytes.NewBufferString(`{"name":"Auth Hub","base_url":"http://127.0.0.1:1","token":"fake-token-0000"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	return resp
}

// readErrorMessage decodes the {"error":{"message"}} envelope.
func readErrorMessage(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var env errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return env.Error.Message
}

// TestAdminAuth covers the ticket 07 acceptance criteria: writes are locked
// behind the admin session, reads stay public, and forged cookies fail.
func TestAdminAuth(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	anon := &http.Client{}

	t.Run("unauthenticated_write_rejected", func(t *testing.T) {
		resp := createHub(t, anon, ts.URL, "")
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		if msg := readErrorMessage(t, resp); msg != "authentication required" {
			t.Fatalf("expected 'authentication required', got %q", msg)
		}
	})

	t.Run("admin_reads_require_session", func(t *testing.T) {
		// Since ticket 16, only the status board (overview + endpoint
		// detail/series/probes + per-model eval summary) is public; admin
		// reads like /api/hubs require a session. The full tier matrix lives
		// in security_test.go.
		resp, err := anon.Get(ts.URL + "/api/hubs")
		if err != nil {
			t.Fatalf("GET /api/hubs: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}

		// The status board stays public.
		resp, err = anon.Get(ts.URL + "/api/overview")
		if err != nil {
			t.Fatalf("GET /api/overview: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		// Per-model eval summary is public too (feeds the public detail page).
		resp, err = anon.Get(ts.URL + "/api/models/1/eval-summary")
		if err != nil {
			t.Fatalf("GET /api/models/1/eval-summary: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatal("eval-summary must not require a session")
		}

		me, err := anon.Get(ts.URL + "/api/auth/me")
		if err != nil {
			t.Fatalf("GET /api/auth/me: %v", err)
		}
		defer me.Body.Close()
		var env envelope
		if err := json.NewDecoder(me.Body).Decode(&env); err != nil {
			t.Fatalf("decode me: %v", err)
		}
		var status map[string]bool
		if err := json.Unmarshal(env.Data, &status); err != nil {
			t.Fatalf("unmarshal me: %v", err)
		}
		if status["authenticated"] {
			t.Fatal("anonymous request must not be authenticated")
		}
	})

	t.Run("wrong_password_rejected", func(t *testing.T) {
		resp := postRaw(t, anon, ts.URL+"/api/auth/login", `{"username":"admin","password":"wrong-password"}`)
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		if msg := readErrorMessage(t, resp); msg != "invalid credentials" {
			t.Fatalf("expected 'invalid credentials', got %q", msg)
		}
	})

	t.Run("login_grants_writes_and_logout_revokes", func(t *testing.T) {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatalf("cookie jar: %v", err)
		}
		client := &http.Client{Jar: jar}

		login := postRaw(t, client, ts.URL+"/api/auth/login",
			`{"username":"admin","password":`+strconv.Quote(testAdminPassword)+`}`)
		login.Body.Close()
		if login.StatusCode != http.StatusOK {
			t.Fatalf("login: expected 200, got %d", login.StatusCode)
		}

		// A write now passes with the session cookie.
		resp := createHub(t, client, ts.URL, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("authenticated write: expected 201, got %d", resp.StatusCode)
		}

		// /me confirms the session and returns the user identity.
		me, err := client.Get(ts.URL + "/api/auth/me")
		if err != nil {
			t.Fatalf("GET /api/auth/me: %v", err)
		}
		var env envelope
		json.NewDecoder(me.Body).Decode(&env)
		me.Body.Close()
		var meResp struct {
			Authenticated bool `json:"authenticated"`
			User          struct {
				ID       int64   `json:"id"`
				Username string  `json:"username"`
				Role     string  `json:"role"`
				HubID    *int64  `json:"hub_id"`
				HubName  *string `json:"hub_name"`
			} `json:"user"`
		}
		if err := json.Unmarshal(env.Data, &meResp); err != nil {
			t.Fatalf("unmarshal me: %v", err)
		}
		if !meResp.Authenticated {
			t.Fatal("logged-in request must be authenticated")
		}
		if meResp.User.Username != "admin" {
			t.Fatalf("username: expected admin, got %q", meResp.User.Username)
		}
		if meResp.User.Role != "super_admin" {
			t.Fatalf("role: expected super_admin, got %q", meResp.User.Role)
		}
		if meResp.User.HubID != nil {
			t.Fatalf("super_admin hub_id must be null, got %v", *meResp.User.HubID)
		}

		// Logout clears the cookie; writes are rejected again.
		logout := postRaw(t, client, ts.URL+"/api/auth/logout", "")
		logout.Body.Close()
		if logout.StatusCode != http.StatusNoContent {
			t.Fatalf("logout: expected 204, got %d", logout.StatusCode)
		}
		resp = createHub(t, client, ts.URL, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("post-logout write: expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("expired_cookie_rejected", func(t *testing.T) {
		// Correctly signed but issued 8 days ago, past the 7-day TTL.
		token := forgeSessionToken(t, 1, time.Now().Add(-8*24*time.Hour), false)
		resp := createHub(t, anon, ts.URL, "ahc_session="+token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("tampered_cookie_rejected", func(t *testing.T) {
		token := forgeSessionToken(t, 1, time.Now(), true)
		resp := createHub(t, anon, ts.URL, "ahc_session="+token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})
}
