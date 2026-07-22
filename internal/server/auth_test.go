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
)

// testAdminPassword is a fixed fake password used by every test server; it is
// not a real credential.
const testAdminPassword = "test-admin-password"

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
		bytes.NewBufferString(`{"password":`+strconv.Quote(testAdminPassword)+`}`))
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
// mint expired or tampered cookies without any server cooperation.
func forgeSessionToken(t *testing.T, issued time.Time, tamper bool) string {
	t.Helper()
	key := sha256.Sum256([]byte("ai-hub-checker-session:" + testAdminPassword))
	stamp := strconv.FormatInt(issued.Unix(), 10)
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(stamp))
	sig := hex.EncodeToString(mac.Sum(nil))
	if tamper {
		replacement := "a"
		if sig[0] == 'a' {
			replacement = "b"
		}
		sig = replacement + sig[1:]
	}
	return stamp + "." + sig
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
		// detail/series/probes) is public; admin reads like /api/hubs
		// require a session. The full tier matrix lives in security_test.go.
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
		resp := postRaw(t, anon, ts.URL+"/api/auth/login", `{"password":"wrong-password"}`)
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
		if msg := readErrorMessage(t, resp); msg != "invalid password" {
			t.Fatalf("expected 'invalid password', got %q", msg)
		}
	})

	t.Run("login_grants_writes_and_logout_revokes", func(t *testing.T) {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatalf("cookie jar: %v", err)
		}
		client := &http.Client{Jar: jar}

		login := postRaw(t, client, ts.URL+"/api/auth/login",
			`{"password":`+strconv.Quote(testAdminPassword)+`}`)
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

		// /me confirms the session.
		me, err := client.Get(ts.URL + "/api/auth/me")
		if err != nil {
			t.Fatalf("GET /api/auth/me: %v", err)
		}
		var env envelope
		json.NewDecoder(me.Body).Decode(&env)
		me.Body.Close()
		var status map[string]bool
		json.Unmarshal(env.Data, &status)
		if !status["authenticated"] {
			t.Fatal("logged-in request must be authenticated")
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
		token := forgeSessionToken(t, time.Now().Add(-8*24*time.Hour), false)
		resp := createHub(t, anon, ts.URL, "ahc_session="+token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("tampered_cookie_rejected", func(t *testing.T) {
		token := forgeSessionToken(t, time.Now(), true)
		resp := createHub(t, anon, ts.URL, "ahc_session="+token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})
}
