package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateTier configures one token-bucket tier: PerMinute is the sustained
// refill rate and Burst the bucket size. A zero tier means unlimited.
type RateTier struct {
	PerMinute float64
	Burst     int
}

// RateLimits bundles the three tiers: the login endpoint (strict, against
// password brute force), writes and manual triggers (moderate), and public
// reads (generous). MaxEntriesPerTier caps each tier's per-IP bucket map
// (0 applies the built-in default); it exists so tests can exercise the cap
// without spraying 100k IPs.
type RateLimits struct {
	Login             RateTier
	Write             RateTier
	Read              RateTier
	MaxEntriesPerTier int
}

// defaultRateLimits are the production tiers: 5 logins/min, 30 writes/min,
// 20 reads/sec per client IP.
func defaultRateLimits() RateLimits {
	return RateLimits{
		Login: RateTier{PerMinute: 5, Burst: 5},
		Write: RateTier{PerMinute: 30, Burst: 10},
		Read:  RateTier{PerMinute: 1200, Burst: 40},
	}
}

// ipLimiter is a per-client-IP token bucket set for one tier.
type ipLimiter struct {
	mu         sync.Mutex
	rps        rate.Limit
	burst      int
	maxEntries int
	items      map[string]*limiterEntry
	now        func() time.Time
	sweeps     int
}

// limiterEntry tracks a bucket and when it was last touched (for eviction).
type limiterEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

// limiterIdleTTL is how long an idle per-IP bucket is kept.
const limiterIdleTTL = 10 * time.Minute

// limiterMaxEntries hard-caps the bucket map. Without it, an IP spray
// (forged X-Forwarded-For under TRUST_PROXY, or IPv6 rotation) could grow
// the map without bound within the idle TTL. When the cap is hit, unknown
// IPs fail closed (429) until the sweep frees entries.
const limiterMaxEntries = 100_000

// newIPLimiter builds a tier; rps<=0 or burst<=0 disables limiting (nil).
// maxEntries<=0 applies the built-in cap.
func newIPLimiter(perMinute float64, burst int, maxEntries int) *ipLimiter {
	if perMinute <= 0 || burst <= 0 {
		return nil
	}
	if maxEntries <= 0 {
		maxEntries = limiterMaxEntries
	}
	return &ipLimiter{
		rps:        rate.Limit(perMinute / 60),
		burst:      burst,
		maxEntries: maxEntries,
		items:      map[string]*limiterEntry{},
		now:        time.Now,
	}
}

// allow reports whether one request from ip may proceed.
func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Evict idle buckets opportunistically every 1024 calls so a spray of
	// distinct IPs cannot grow the map without bound.
	l.sweeps++
	if l.sweeps >= 1024 {
		l.sweeps = 0
		cutoff := l.now().Add(-limiterIdleTTL)
		for ip, e := range l.items {
			if e.seen.Before(cutoff) {
				delete(l.items, ip)
			}
		}
	}

	e, ok := l.items[ip]
	if !ok {
		// Hard cap: fail closed while the table is full rather than letting
		// an IP spray exhaust memory.
		if len(l.items) >= l.maxEntries {
			return false
		}
		e = &limiterEntry{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.items[ip] = e
	}
	e.seen = l.now()
	return e.limiter.Allow()
}

// tierFor picks the limiter tier for a request: login attempts first (they
// are POSTs but get the strict bucket), reads (GET/HEAD) second, everything
// else is a write.
func (s *Server) tierFor(r *http.Request) *ipLimiter {
	if r.Method == http.MethodPost && r.URL.Path == "/api/auth/login" {
		return s.loginLimiter
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return s.readLimiter
	}
	return s.writeLimiter
}

// rateLimit rejects requests over their tier's budget with 429.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tier := s.tierFor(r); tier != nil && !tier.allow(s.clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded, please slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP resolves the client host for rate limiting and auditing. The
// first X-Forwarded-For hop is honored only when the server is configured to
// trust a forwarding proxy; otherwise the header is ignored (it is freely
// spoofable). NOTE for operators: the fronting proxy must REPLACE any
// client-supplied X-Forwarded-For header, not append to it — otherwise the
// forged leftmost hop becomes authoritative here.
func (s *Server) clientIP(r *http.Request) string {
	if s.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first, _, _ := strings.Cut(xff, ","); strings.TrimSpace(first) != "" {
				return strings.TrimSpace(first)
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// securityHeaders sets baseline hardening headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// style-src allows inline styles because Element Plus renders style
		// attributes; scripts and frames stay self-only.
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'self'; base-uri 'self'")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// maxBodyBytes caps JSON request bodies.
const maxBodyBytes = 1 << 20

// limitBodySize rejects oversized request bodies (decode fails with 400).
func limitBodySize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}
