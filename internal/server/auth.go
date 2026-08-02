package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/taliove/hubscope/internal/store"
)

// sessionCookieName is the name of the session cookie.
const sessionCookieName = "ahc_session"

// sessionTTL bounds how long a session cookie stays valid.
const sessionTTL = 7 * 24 * time.Hour

// loadSessionSecret resolves the HMAC signing key for session cookies. It
// checks the SESSION_SECRET env var first, then the settings table; when
// neither yields a secret, it generates a random 32-byte value and persists
// it (hex-encoded) so subsequent restarts reuse the same key. The secret is
// independent of any password — rotating it invalidates all sessions.
func loadSessionSecret(db *store.DB) []byte {
	if envSecret := os.Getenv("SESSION_SECRET"); envSecret != "" {
		return []byte(envSecret)
	}
	stored, err := db.GetSetting(store.SettingSessionSecret, "")
	if err == nil && stored != "" {
		if raw, derr := hex.DecodeString(stored); derr == nil && len(raw) > 0 {
			return raw
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("server: failed to generate session secret: " + err.Error())
	}
	if err := db.SetSetting(store.SettingSessionSecret, hex.EncodeToString(raw)); err != nil {
		panic("server: failed to persist session secret: " + err.Error())
	}
	return raw
}

// signSession builds a signed session token of the form
// "<userId>.<issuedUnix>.<hmacHex>". The HMAC covers "userId.issuedUnix" so
// the user identity is bound to the signature and cannot be swapped.
func signSession(key []byte, userID int64, issued time.Time) string {
	uid := strconv.FormatInt(userID, 10)
	stamp := strconv.FormatInt(issued.Unix(), 10)
	payload := uid + "." + stamp
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifySession checks the signature and expiry of a session token. On
// success it returns the userID embedded in the token; on any failure it
// returns 0 and false.
func verifySession(key []byte, token string, now time.Time) (int64, bool) {
	uidStr, stamp, sigHex, ok := splitThree(token)
	if !ok {
		return 0, false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(uidStr + "." + stamp))
	sig, err := hex.DecodeString(sigHex)
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, false
	}
	userID, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil {
		return 0, false
	}
	issued, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return 0, false
	}
	age := now.Sub(time.Unix(issued, 0))
	if age < 0 || age > sessionTTL {
		return 0, false
	}
	return userID, true
}

// splitThree splits "a.b.c" into its three non-empty parts.
func splitThree(token string) (string, string, string, bool) {
	first, rest, ok := strings.Cut(token, ".")
	if !ok || first == "" {
		return "", "", "", false
	}
	second, third, ok := strings.Cut(rest, ".")
	if !ok || second == "" || third == "" {
		return "", "", "", false
	}
	return first, second, third, true
}

// isHTTPS reports whether the request arrived over TLS (directly or via a
// forwarding proxy), in which case cookies may carry the Secure attribute.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// setSessionCookie issues the session cookie for a successful login.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   int(sessionTTL.Seconds()),
		Expires:  time.Now().Add(sessionTTL),
	})
}

// clearSessionCookie expires the session cookie on the client.
func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// handleLogin validates username/password against the users table and issues
// a session cookie. The error message is uniform ("invalid credentials") for
// unknown user, wrong password, and disabled accounts to prevent username
// enumeration; the audit log records the specific failure reason.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		CaptchaID     string `json:"captcha_id"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Adaptive captcha (spec 0012): when either the source IP or the
	// username dimension is armed, the attempt must carry a valid captcha
	// BEFORE any account lookup or password work. The verification runs
	// ahead of the progressive-delay penalize sleep, so captcha-stage
	// rejections never make the user wait 2-8s; they also never count
	// toward the delay (no password was attempted) — but they do feed the
	// audit trail and the instance-level brute-force tracker, same as any
	// failed login (spec 0012 decision 7). Captcha fields on an attempt
	// that is not required are ignored outright (old clients unaffected).
	ip := s.clientIP(r)
	if s.captchaTrigger != nil && s.captchaTrigger.required(ip, req.Username) {
		if req.CaptchaID == "" || req.CaptchaAnswer == "" {
			s.audit(r, "auth.login", "auth", req.Username, "captcha required", "failed")
			if s.loginAlertTracker != nil {
				s.loginAlertTracker.record(req.Username, ip)
			}
			writeCaptchaError(w, http.StatusUnauthorized, "请完成验证码")
			return
		}
		if s.captchaStore == nil || !s.captchaStore.verify(req.CaptchaID, req.CaptchaAnswer) {
			s.audit(r, "auth.login", "auth", req.Username, "captcha wrong", "failed")
			if s.loginAlertTracker != nil {
				s.loginAlertTracker.record(req.Username, ip)
			}
			writeCaptchaError(w, http.StatusUnauthorized, "验证码错误或已过期")
			return
		}
		// Captcha passed (and is destroyed — one-time use). Fall through to
		// the normal password path; the frontend re-issues after every
		// attempt.
	}
	user, err := s.db.GetUserByUsername(req.Username)
	if err != nil {
		s.failLogin(w, r, req.Username, "user not found")
		return
	}
	// Inject the attempted identity into the request context so the audit
	// calls below record the real username as actor. The login endpoint
	// sits outside requireSession, so without this the disabled / wrong-
	// password / success audits would all read "system". The "user not found"
	// branch above has no user to inject and keeps the "system" fallback.
	r = r.WithContext(withSessionUser(r.Context(), SessionUser{
		UserID:   user.ID,
		Role:     user.Role,
		HubID:    user.HubID,
		Username: user.Username,
	}))
	if !user.Enabled {
		s.failLogin(w, r, req.Username, "disabled")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.failLogin(w, r, req.Username, "wrong password")
		return
	}
	// Correct password: always admitted immediately (verify first, penalize
	// later — the delay only ever lands on wrong-password responses), and the
	// account's failure counter resets.
	if s.loginDelayer != nil {
		s.loginDelayer.reset(req.Username)
	}
	// A successful login also disarms the captcha trigger for both the
	// username and the source IP (spec 0012 decision 1); the two counters
	// reset independently of each other.
	if s.captchaTrigger != nil {
		s.captchaTrigger.reset(ip, req.Username)
	}
	setSessionCookie(w, r, signSession(s.sessionSecret, user.ID, time.Now()))
	s.audit(r, "auth.login", "auth", req.Username, "", "success")
	writeData(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// failLogin is the single exit for every failed login (unknown user,
// disabled account, wrong password): it audits the specific reason, then
// feeds the instance-level brute-force tracker, then applies the per-account
// progressive delay, then answers the uniform 401. Routing all three
// branches through one exit keeps status, message, and delay band identical
// across them — no username-enumeration side channel (spec 0011 decision 3).
// The delay runs after the audit insert, so the sleep never holds any DB
// resource (single-connection SQLite, W2). The tracker step is a pure
// in-memory count and runs before the delay: penalize's sleep must not
// postpone the alert (spec 0011 decision 4).
func (s *Server) failLogin(w http.ResponseWriter, r *http.Request, username, reason string) {
	s.audit(r, "auth.login", "auth", username, reason, "failed")
	if s.loginAlertTracker != nil {
		s.loginAlertTracker.record(username, s.clientIP(r))
	}
	// Feed the adaptive captcha trigger (spec 0012 decision 1) on the same
	// failure observation point: pure in-memory count, before the penalize
	// sleep. Captcha-stage failures never reach this exit — they are
	// rejected in handleLogin and deliberately do not count here.
	if s.captchaTrigger != nil {
		s.captchaTrigger.recordFailure(s.clientIP(r), username)
	}
	if s.loginDelayer != nil {
		s.loginDelayer.penalize(r.Context(), username)
	}
	writeError(w, http.StatusUnauthorized, "invalid credentials")
}

// handleLogout clears the session cookie. The /auth group is not gated by
// requireSession, so the SessionUser is injected here from the cookie so the
// logout audit records the real actor rather than the "system" fallback.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if userID, ok := s.hasValidSession(r); ok {
		if user, err := s.db.GetUserByID(userID); err == nil && user.Enabled {
			r = r.WithContext(withSessionUser(r.Context(), SessionUser{
				UserID:   user.ID,
				Role:     user.Role,
				HubID:    user.HubID,
				Username: user.Username,
			}))
		}
	}
	clearSessionCookie(w, r)
	s.audit(r, "auth.logout", "auth", "", "", "success")
	writeNoContent(w)
}

// handleAuthMe reports whether the request carries a valid session and, when
// it does, returns the user identity. Public.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.hasValidSession(r)
	if !ok {
		writeData(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
			"user":          nil,
		})
		return
	}
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		// Valid cookie but user no longer exists (deleted) — treat the
		// session as unauthenticated rather than erroring.
		writeData(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
			"user":          nil,
		})
		return
	}
	var hubName *string
	if user.HubID != nil {
		if hub, herr := s.db.GetHub(*user.HubID); herr == nil {
			name := hub.Name
			hubName = &name
		}
	}
	writeData(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"hub_id":   user.HubID,
			"hub_name": hubName,
		},
	})
}

// hasValidSession reports whether the request carries a valid session cookie
// and, when it does, returns the embedded userID.
func (s *Server) hasValidSession(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, false
	}
	return verifySession(s.sessionSecret, cookie.Value, time.Now())
}

// publicReadPattern matches the read paths that stay public: the status
// board (overview matrix, endpoint detail/series/probes, and the per-model
// eval summary that feeds the public endpoint detail page), the public
// alert timeline (spec 0019 — the anonymous payload is filtered to the
// publicAlertKinds whitelist, so only the four incident-narrative kinds are
// exposed), the public eval board (spec 0010 — the newest settled
// campaign's report, same information level as the shared report) plus the
// token-gated shared report itself (ADR 0006 — the token in the path is the
// credential, and the handler answers unknown/revoked tokens with a uniform
// 404). Every other GET requires a session, like all writes.
var publicReadPattern = regexp.MustCompile(`^/api/(overview|alerts|endpoints/\d+(/series|/probes)?|models/\d+/eval-summary|shared-reports/[^/]+|public/eval/board)$`)

// publicAlertKinds is the anonymous whitelist for GET /api/alerts
// (spec 0019, ui-guidelines appendix item 16): the four incident-narrative
// kinds. The seven operational-pipeline kinds (test / batch /
// quiet_summary / score_drop / score_drop_skipped / retire_pending /
// retired) — and with them every hub-less event, which only those kinds
// produce — stay session-only. The constant lives next to
// publicReadPattern so the public information boundary reads at one point.
var publicAlertKinds = []string{
	store.AlertKindDown,
	store.AlertKindRecovered,
	store.AlertKindGroupDown,
	store.AlertKindGroupRecovered,
}

// requireSession rejects requests without a valid session cookie, except
// status-board GETs, which stay public by design. On a valid session it
// loads the user (role, hub scope, username) and injects it into the request
// context via withSessionUser, so downstream handlers and s.audit can read
// the identity without re-querying. A user whose account has been disabled
// since the cookie was issued is rejected with 401 (same message as an
// unauthenticated request) so disabled accounts cannot be probed and the
// window between revocation and token expiry is closed. The public-read
// bypass path does not load the user and is unaffected by the Enabled flag
// (public reads are public regardless of who asks).
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := s.hasValidSession(r); ok {
			user, err := s.db.GetUserByID(userID)
			if err != nil || !user.Enabled {
				// Unknown user (deleted) or disabled since the cookie was
				// issued: treat as unauthenticated. Same message as below to
				// avoid account-state probing.
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			r = r.WithContext(withSessionUser(r.Context(), SessionUser{
				UserID:   user.ID,
				Role:     user.Role,
				HubID:    user.HubID,
				Username: user.Username,
			}))
			next.ServeHTTP(w, r)
			return
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && publicReadPattern.MatchString(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "authentication required")
	})
}
