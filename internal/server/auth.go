package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// sessionCookieName is the name of the admin session cookie.
const sessionCookieName = "ahc_session"

// sessionTTL bounds how long a session cookie stays valid.
const sessionTTL = 7 * 24 * time.Hour

// deriveSessionKey derives the HMAC-SHA256 signing key from the admin
// password, so no separate secret needs to be configured or stored.
func deriveSessionKey(adminPassword string) []byte {
	sum := sha256.Sum256([]byte("hubscope-session:" + adminPassword))
	return sum[:]
}

// signSession builds a signed session token of the form "<issuedUnix>.<hmacHex>".
func signSession(key []byte, issued time.Time) string {
	stamp := strconv.FormatInt(issued.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(stamp))
	return stamp + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifySession checks the signature and the expiry of a session token.
func verifySession(key []byte, token string, now time.Time) bool {
	stamp, sigHex, ok := strings.Cut(token, ".")
	if !ok || stamp == "" || sigHex == "" {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(stamp))
	sig, err := hex.DecodeString(sigHex)
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return false
	}
	issued, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return false
	}
	age := now.Sub(time.Unix(issued, 0))
	return age >= 0 && age <= sessionTTL
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

// handleLogin validates the admin password and issues a session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.adminPassword)) != 1 {
		s.audit(r, "auth.login", "auth", "", "", "failed")
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	setSessionCookie(w, r, signSession(s.sessionKey, time.Now()))
	s.audit(r, "auth.login", "auth", "", "", "success")
	writeData(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// handleLogout clears the session cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, r)
	s.audit(r, "auth.logout", "auth", "", "", "success")
	writeNoContent(w)
}

// handleAuthMe reports whether the request carries a valid session. Public.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]bool{"authenticated": s.hasValidSession(r)})
}

// hasValidSession reports whether the request carries a valid session cookie.
func (s *Server) hasValidSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return verifySession(s.sessionKey, cookie.Value, time.Now())
}

// publicReadPattern matches the read paths that stay public: the status
// board (overview matrix and endpoint detail/series/probes) plus the
// token-gated shared report (ADR 0006 — the token in the path is the
// credential, and the handler answers unknown/revoked tokens with a uniform
// 404). Every other GET requires a session, like all writes.
var publicReadPattern = regexp.MustCompile(`^/api/(overview|endpoints/\d+(/series|/probes)?|shared-reports/[^/]+)$`)

// requireSession rejects requests without a valid session cookie, except
// status-board GETs, which stay public by design.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.hasValidSession(r) {
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
