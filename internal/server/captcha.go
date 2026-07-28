package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mojocn/base64Captcha"
)

// CaptchaPolicy configures the adaptive captcha trigger (spec 0012 decision
// 1): after Threshold failed logins for the same source IP or the same
// username string within a sliding Window, subsequent logins touching that
// dimension must carry a valid captcha. The two dimensions are independent;
// either one armed is enough to require. MaxEntries hard-caps each dimension
// map (0 applies the built-in default). The zero policy disables the
// mechanism (nil trigger), mirroring the loginDelayer nil semantics.
type CaptchaPolicy struct {
	Threshold  int
	Window     time.Duration
	MaxEntries int
}

// defaultCaptchaPolicy is the production policy: 2 failures within 10
// minutes, captcha required from the 3rd attempt on (spec 0012 decision 1).
func defaultCaptchaPolicy() CaptchaPolicy {
	return CaptchaPolicy{
		Threshold: 2,
		Window:    10 * time.Minute,
	}
}

// captchaTriggerMaxEntries hard-caps each dimension map. Without it, an
// IP/username spray could grow the maps without bound. Unlike the fail-
// closed per-IP limiter, a full table skips counting for new keys (the
// per-IP login limiter remains the backstop) rather than locking out
// legitimate logins — the loginDelayer precedent.
const captchaTriggerMaxEntries = 100_000

// captchaTrigger counts failed logins per source IP and per username string
// in memory (unknown usernames count too, so behavior cannot leak account
// existence — spec 0011 decision 3's observation point, shared per spec
// 0012 decision 1). The count only answers "captcha required?"; the
// loginDelayer's count only answers "how long to penalize" — same failure
// observation point, independent semantics and independent reset paths
// (spec 0012 note). Captcha-stage failures deliberately do NOT count: the
// dimension is already armed (count >= threshold), so counting would change
// no behavior, and letting an idle 10-minute window disarm the requirement
// is the designed adaptive semantics. State is process-local and lost on
// restart — an accepted trade-off, same as the loginDelayer.
type captchaTrigger struct {
	mu         sync.Mutex
	threshold  int
	window     time.Duration
	maxEntries int
	keep       int // per-entry timestamp cap: threshold + 1
	byIP       map[string]*captchaTriggerEntry
	byUser     map[string]*captchaTriggerEntry
	sweeps     int
}

// captchaTriggerEntry tracks recent failure timestamps for one key.
type captchaTriggerEntry struct {
	failures []time.Time
	seen     time.Time
}

// newCaptchaTrigger builds the counter; a zero policy disables the mechanism
// (nil). maxEntries<=0 applies the built-in cap.
func newCaptchaTrigger(p CaptchaPolicy) *captchaTrigger {
	if p.Threshold <= 0 || p.Window <= 0 {
		return nil
	}
	if p.MaxEntries <= 0 {
		p.MaxEntries = captchaTriggerMaxEntries
	}
	return &captchaTrigger{
		threshold:  p.Threshold,
		window:     p.Window,
		maxEntries: p.MaxEntries,
		keep:       p.Threshold + 1,
		byIP:       map[string]*captchaTriggerEntry{},
		byUser:     map[string]*captchaTriggerEntry{},
	}
}

// required reports whether either dimension is armed for this attempt.
func (t *captchaTrigger) required(ip, username string) bool {
	cutoff := time.Now().Add(-t.window)
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.armedLocked(t.byIP, ip, cutoff) || t.armedLocked(t.byUser, username, cutoff)
}

// armedLocked reports whether key has at least threshold failures inside the
// window. Callers hold mu.
func (t *captchaTrigger) armedLocked(m map[string]*captchaTriggerEntry, key string, cutoff time.Time) bool {
	e, ok := m[key]
	if !ok {
		return false
	}
	count := 0
	for _, ts := range e.failures {
		if ts.After(cutoff) {
			count++
		}
	}
	return count >= t.threshold
}

// recordFailure notes one failed login in both dimensions. Pure in-memory
// step (microseconds, no DB resource) so it can run on the login request
// path before the progressive-delay sleep.
func (t *captchaTrigger) recordFailure(ip, username string) {
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()
	// Evict stale entries opportunistically every 1024 calls so a spray
	// cannot grow the maps without bound (the loginDelayer precedent).
	t.sweeps++
	if t.sweeps >= 1024 {
		t.sweeps = 0
		cutoff := now.Add(-t.window)
		for _, m := range []map[string]*captchaTriggerEntry{t.byIP, t.byUser} {
			for key, e := range m {
				if e.seen.Before(cutoff) {
					delete(m, key)
				}
			}
		}
	}
	t.recordLocked(t.byIP, ip, now)
	t.recordLocked(t.byUser, username, now)
}

// recordLocked appends one failure for key, sliding the window. Callers
// hold mu.
func (t *captchaTrigger) recordLocked(m map[string]*captchaTriggerEntry, key string, now time.Time) {
	e, ok := m[key]
	if !ok {
		if len(m) >= t.maxEntries {
			// Table full: skip counting rather than failing closed.
			return
		}
		e = &captchaTriggerEntry{}
		m[key] = e
	}
	cutoff := now.Add(-t.window)
	kept := e.failures[:0]
	for _, ts := range e.failures {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	e.failures = append(kept, now)
	// Cap the per-entry slice: past threshold+1 the exact count no longer
	// changes the boolean answer, and an uncapped slice would let a single
	// sprayed key bypass the map's memory bound.
	if len(e.failures) > t.keep {
		e.failures = e.failures[len(e.failures)-t.keep:]
	}
	e.seen = now
}

// reset clears both dimensions after a successful login (spec 0012
// decision 1); the loginDelayer's reset is independent of this one.
func (t *captchaTrigger) reset(ip, username string) {
	t.mu.Lock()
	delete(t.byIP, ip)
	delete(t.byUser, username)
	t.mu.Unlock()
}

// CaptchaStorePolicy configures the answer store (spec 0012 decision 2):
// answers live in memory keyed by a high-entropy random id, expire after
// TTL, and are destroyed by any single verification attempt. MaxEntries
// hard-caps the store (0 applies the built-in default); a full store fails
// closed — issuing stops with 503 rather than silently letting clients skip
// the captcha. Non-positive TTL applies the built-in default.
type CaptchaStorePolicy struct {
	TTL        time.Duration
	MaxEntries int
}

// captchaStoreDefaultTTL is the production answer lifetime (spec 0012
// decision 2).
const captchaStoreDefaultTTL = 5 * time.Minute

// captchaStoreDefaultMaxEntries hard-caps the answer map. Sizing (ticket 88
// risk 2): one IP can hold at most ~100 live entries (20/min issue budget ×
// 5-minute TTL), so 10k entries absorbs ~100 concurrently saturated source
// IPs; the 503 blast radius is limited to clients already required to solve
// a captcha — clean users never touch this path. Memory: ~1.5MB at the cap.
const captchaStoreDefaultMaxEntries = 10_000

// errCaptchaStoreFull is returned by issue when the store is at capacity;
// the handler answers 503 (fail-closed).
var errCaptchaStoreFull = errors.New("captcha store full")

// captchaEntry is one issued answer awaiting verification.
type captchaEntry struct {
	answer  string
	expires time.Time
}

// CaptchaStore holds issued captcha answers in memory. State is
// process-local and lost on restart — clients simply re-issue, matching the
// other in-memory defenses (spec 0012 decision 1's accepted bound).
type CaptchaStore struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	items      map[string]captchaEntry
}

// NewCaptchaStore builds the store; non-positive TTL / MaxEntries apply the
// built-in defaults.
func NewCaptchaStore(p CaptchaStorePolicy) *CaptchaStore {
	if p.TTL <= 0 {
		p.TTL = captchaStoreDefaultTTL
	}
	if p.MaxEntries <= 0 {
		p.MaxEntries = captchaStoreDefaultMaxEntries
	}
	return &CaptchaStore{
		ttl:        p.TTL,
		maxEntries: p.MaxEntries,
		items:      map[string]captchaEntry{},
	}
}

// issue mints a fresh captcha: a high-entropy id and a 6-digit answer, both
// from crypto/rand (the library's RandomId uses math/rand and is banned —
// W6). Expired entries are swept on every issue; the per-IP issue rate
// limit keeps that sweep cheap. A full store fails closed.
func (st *CaptchaStore) issue() (id, answer string, err error) {
	now := time.Now()

	st.mu.Lock()
	// Sweep expired entries before the capacity check so a backlog of dead
	// entries cannot trip the fail-closed path.
	for key, e := range st.items {
		if now.After(e.expires) {
			delete(st.items, key)
		}
	}
	if len(st.items) >= st.maxEntries {
		st.mu.Unlock()
		return "", "", errCaptchaStoreFull
	}
	id, err = randomCaptchaID()
	if err != nil {
		st.mu.Unlock()
		return "", "", err
	}
	answer, err = randomCaptchaAnswer()
	if err != nil {
		st.mu.Unlock()
		return "", "", err
	}
	st.items[id] = captchaEntry{answer: answer, expires: now.Add(st.ttl)}
	st.mu.Unlock()
	return id, answer, nil
}

// verify checks one answer and ALWAYS destroys the entry — one-time use,
// whether the answer was right, wrong, or expired (spec 0012 decision 2).
func (st *CaptchaStore) verify(id, answer string) bool {
	st.mu.Lock()
	e, ok := st.items[id]
	if ok {
		delete(st.items, id)
	}
	st.mu.Unlock()
	if !ok || time.Now().After(e.expires) {
		return false
	}
	return e.answer == answer
}

// Seed inserts a known id/answer pair with the store's TTL. It is the test
// seam that lets black-box tests carry a correct answer through the HTTP
// API without reading store internals (the WithLoginDelay precedent).
func (st *CaptchaStore) Seed(id, answer string) {
	st.mu.Lock()
	st.items[id] = captchaEntry{answer: answer, expires: time.Now().Add(st.ttl)}
	st.mu.Unlock()
}

// randomCaptchaID returns a 128-bit hex id from crypto/rand.
func randomCaptchaID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// randomCaptchaAnswer returns a 6-digit answer from crypto/rand.
func randomCaptchaAnswer() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	digits := make([]byte, 6)
	for i, b := range raw {
		digits[i] = '0' + b%10
	}
	return string(digits), nil
}

// handleCaptcha issues one captcha as a PNG data URI (spec 0012 decision
// 2). Public (pre-login, no session) and rate-limited on its own tier
// (spec 0012 decision 4). The digit driver renders from a bitmap font
// compiled into the binary — zero external font files (W8). A full answer
// store answers 503 fail-closed: silently skipping would hand attackers a
// captcha-free channel.
func (s *Server) handleCaptcha(w http.ResponseWriter, r *http.Request) {
	if s.captchaStore == nil {
		writeError(w, http.StatusServiceUnavailable, "验证码服务暂不可用,请稍后重试")
		return
	}
	id, answer, err := s.captchaStore.issue()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "验证码服务暂不可用,请稍后重试")
		return
	}
	item, err := base64Captcha.NewDriverDigit(48, 160, 6, 0.6, 40).DrawCaptcha(answer)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "验证码服务暂不可用,请稍后重试")
		return
	}
	writeData(w, http.StatusOK, map[string]string{
		"captcha_id": id,
		"image":      item.EncodeB64string(),
	})
}

// writeCaptchaError writes the login failure envelope extended with the
// machine-readable captcha_required marker (spec 0012 decision 3 — the
// frontend unfolds the captcha section on this flag; the field set and the
// message wording are the frozen contract for ticket 89).
func writeCaptchaError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":          message,
			"captcha_required": true,
		},
	})
}
