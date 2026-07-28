package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/taliove/hubscope/internal/alerter"
	"github.com/taliove/hubscope/internal/discovery"
	"github.com/taliove/hubscope/internal/evaluator"
	"github.com/taliove/hubscope/internal/hubclient"
	"github.com/taliove/hubscope/internal/prober"
	"github.com/taliove/hubscope/internal/store"
)

// Server holds dependencies and the HTTP router.
type Server struct {
	db        *store.DB
	prober    *prober.Prober
	discovery *discovery.Syncer
	evaluator *evaluator.Evaluator
	alerter   *alerter.Evaluator
	router    chi.Router
	staticFS  fs.FS
	now       func() time.Time
	version   string

	// Rate-limit tiers; a nil limiter means its tier is unlimited.
	loginLimiter *ipLimiter
	writeLimiter *ipLimiter
	readLimiter  *ipLimiter
	// captchaLimiter is the independent tier for the captcha issue endpoint
	// (spec 0012 decision 4: 20/min burst 10 — the login tier is too strict
	// for image-load retries, the read tier would be a free solving range).
	captchaLimiter *ipLimiter
	// loginDelayer penalizes per-account repeated login failures with a
	// progressive response delay (spec 0011 decision 3); nil disables it.
	loginDelayer *loginDelayer
	// loginAlertTracker watches instance-wide login failures and fires one
	// Lark alert per cooldown once the burst threshold is crossed (spec
	// 0011 decision 4); nil disables it.
	loginAlertTracker *loginAlertTracker
	// captchaTrigger arms the adaptive captcha requirement per source IP
	// and per username (spec 0012 decision 1); nil disables it.
	captchaTrigger *captchaTrigger
	// captchaStore holds issued captcha answers in memory (one-time use,
	// TTL, fail-closed cap — spec 0012 decision 2).
	captchaStore *CaptchaStore
	// trustProxy controls whether X-Forwarded-For is honored when resolving
	// client IPs for rate limiting and auditing.
	trustProxy bool

	// sessionSecret signs the stateless session cookie. It is loaded from
	// the SESSION_SECRET env var or the settings table (auto-generated on
	// first start), never from a password. Rotating it invalidates all
	// sessions.
	sessionSecret []byte
}

// Option customizes a Server at construction time.
type Option func(*Server)

// WithNow overrides the server's clock. Tests use it to evaluate time-window
// statistics against seeded probe history at controlled timestamps.
func WithNow(now func() time.Time) Option {
	return func(s *Server) {
		s.now = now
	}
}

// WithRateLimits overrides the per-IP rate-limit tiers. Zero tiers leave
// that class of traffic unlimited (used by tests).
func WithRateLimits(limits RateLimits) Option {
	return func(s *Server) {
		s.loginLimiter = newIPLimiter(limits.Login.PerMinute, limits.Login.Burst, limits.MaxEntriesPerTier)
		s.writeLimiter = newIPLimiter(limits.Write.PerMinute, limits.Write.Burst, limits.MaxEntriesPerTier)
		s.readLimiter = newIPLimiter(limits.Read.PerMinute, limits.Read.Burst, limits.MaxEntriesPerTier)
		s.captchaLimiter = newIPLimiter(limits.Captcha.PerMinute, limits.Captcha.Burst, limits.MaxEntriesPerTier)
	}
}

// WithTrustProxy controls whether X-Forwarded-For is trusted when resolving
// client IPs. Enable only behind a forwarding proxy that sets the header.
func WithTrustProxy(trust bool) Option {
	return func(s *Server) {
		s.trustProxy = trust
	}
}

// WithLoginDelay overrides the per-account progressive login-delay policy
// (spec 0011 decision 3). A zero policy disables the delay; tests inject a
// millisecond-scale table so backoff stays observable without slowing the
// suite (the WithRateLimits precedent).
func WithLoginDelay(policy LoginDelayPolicy) Option {
	return func(s *Server) {
		s.loginDelayer = newLoginDelayer(policy)
	}
}

// WithLoginAlert overrides the instance-level brute-force login-alert
// policy (spec 0011 decision 4). A zero policy disables the mechanism;
// tests inject a millisecond-scale window/cooldown so the debounce stays
// observable without slowing the suite (the WithLoginDelay precedent).
func WithLoginAlert(policy LoginAlertPolicy) Option {
	return func(s *Server) {
		s.loginAlertTracker = newLoginAlertTracker(policy, s.alerter.HandleLoginFailures)
	}
}

// WithSessionSecret overrides the session signing key. Tests use it to
// inject a fixed value so forged tokens can be reproduced without reading
// the database.
func WithSessionSecret(secret []byte) Option {
	return func(s *Server) {
		s.sessionSecret = secret
	}
}

// WithCaptchaPolicy overrides the adaptive captcha trigger policy (spec 0012
// decision 1). A zero policy disables the trigger; tests inject a
// small-threshold policy so the requirement is observable within a few
// attempts (the WithLoginDelay precedent).
func WithCaptchaPolicy(policy CaptchaPolicy) Option {
	return func(s *Server) {
		s.captchaTrigger = newCaptchaTrigger(policy)
	}
}

// WithCaptchaStore overrides the captcha answer store. Tests inject a store
// with a short TTL / small cap and seed known answers through its Seed seam
// (the WithLoginDelay precedent).
func WithCaptchaStore(store *CaptchaStore) Option {
	return func(s *Server) {
		s.captchaStore = store
	}
}

// WithVersion sets the server version string (displayed in the UI footer).
func WithVersion(version string) Option {
	return func(s *Server) {
		s.version = version
	}
}

// New builds a Server with all API routes registered. The session signing
// key is resolved from the SESSION_SECRET env var or the settings table.
func New(db *store.DB, opts ...Option) *Server {
	limits := defaultRateLimits()
	s := &Server{
		db:            db,
		prober:        prober.New(db, hubclient.New()),
		discovery:     discovery.New(db, hubclient.New()),
		evaluator:     evaluator.New(db, hubclient.NewWithTimeout(evaluator.RequestTimeout)),
		alerter:       alerter.NewEvaluator(db, alerter.NewLarkSender()),
		sessionSecret: loadSessionSecret(db),
		now:           time.Now,
		loginLimiter:  newIPLimiter(limits.Login.PerMinute, limits.Login.Burst, 0),
		writeLimiter:  newIPLimiter(limits.Write.PerMinute, limits.Write.Burst, 0),
		readLimiter:   newIPLimiter(limits.Read.PerMinute, limits.Read.Burst, 0),
		loginDelayer:  newLoginDelayer(defaultLoginDelayPolicy()),
		// Adaptive captcha (spec 0012): production trigger + answer store;
		// the issue tier below comes from the same limits bundle.
		captchaLimiter: newIPLimiter(limits.Captcha.PerMinute, limits.Captcha.Burst, 0),
		captchaTrigger: newCaptchaTrigger(defaultCaptchaPolicy()),
		captchaStore:   NewCaptchaStore(CaptchaStorePolicy{}),
	}
	// Alert evaluation hooks into every probe round served by this prober.
	// main hooks the scheduler's prober into the same evaluator via Alerter().
	s.prober.AfterRound = s.alerter.HandleRound
	// Score-drop checks hook into every settled eval campaign served by this
	// evaluator; the weekly eval worker shares it via Evaluator().
	s.evaluator.AfterCampaign = s.alerter.HandleCampaign
	// Brute-force login alerts fire through the same single Evaluator
	// instance (W5), throttled by the server-side tracker.
	s.loginAlertTracker = newLoginAlertTracker(defaultLoginAlertPolicy(), s.alerter.HandleLoginFailures)
	for _, opt := range opts {
		opt(s)
	}
	s.router = s.routes()
	return s
}

// ServeHTTP makes Server an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Discovery exposes the model-discovery syncer so main can run it on a
// periodic schedule.
func (s *Server) Discovery() *discovery.Syncer {
	return s.discovery
}

// Alerter exposes the shared alert evaluator so main can hook the
// scheduler's prober into the same instance (one alerted-state map per
// process, avoiding duplicate alerts from manual vs scheduled rounds).
func (s *Server) Alerter() *alerter.Evaluator {
	return s.alerter
}

// Evaluator exposes the shared eval runner so main can drive the weekly
// eval worker through the same instance (one judge-model resolution and one
// score-drop hook per process).
func (s *Server) Evaluator() *evaluator.Evaluator {
	return s.evaluator
}

// routes wires up all API endpoints plus the health check.
func (s *Server) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	r.Get("/api/version", s.handleVersion)

	r.Route("/api", func(r chi.Router) {
		r.Use(s.rateLimit)
		r.Use(limitBodySize)
		// Auth endpoints stay open; login issues the session cookie.
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleAuthMe)
			// Captcha issue is public (pre-login) on its own rate tier.
			r.Get("/captcha", s.handleCaptcha)
		})

		// All other API routes require a session, except the read paths
		// whitelisted in publicReadPattern (status board, public eval
		// board, token-gated shared report).
		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)

			// Super_admin-only writes: global resources (hubs, settings,
			// classification rules, cases) and hub creation are reserved for
			// super_admin (spec 0005 decisions 1+6). Suites have no write
			// route (immutable/seeded, W7); cases write the global case
			// library so they stay here. evals.create uses the library to run
			// an assessment against a hub's models — a hub-scoped operation,
			// so it lives in the hub-scoped group below.
			r.Group(func(r chi.Router) {
				r.Use(s.requireRole(store.RoleSuperAdmin))

				r.Post("/hubs", s.handleCreateHub)
				r.Put("/hubs/{id}", s.handleUpdateHub)
				r.Delete("/hubs/{id}", s.handleDeleteHub)
				r.Post("/hubs/{id}/sync", s.handleSyncHub)

				r.Post("/classification-rules", s.handleCreateClassificationRule)
				r.Patch("/classification-rules/{id}", s.handlePatchClassificationRule)
				r.Delete("/classification-rules/{id}", s.handleDeleteClassificationRule)

				r.Post("/cases", s.handleCreateCase)
				r.Patch("/cases/{id}", s.handlePatchCase)

				r.Put("/settings", s.handlePutSettings)
			})

			// Hub-scoped writes: super_admin + admin + operator. These act on
			// resources that belong to a hub (models, endpoints, discovery,
			// eval runs, share links) rather than global config.
			r.Group(func(r chi.Router) {
				r.Use(s.requireRole(store.RoleSuperAdmin, store.RoleAdmin, store.RoleOperator))

				r.Post("/models", s.handleCreateModel)
				r.Delete("/models/{id}", s.handleDeleteModel)
				r.Post("/models/{id}/trial", s.handleTrialModel)

				r.Patch("/endpoints/{id}", s.handlePatchEndpoint)
				r.Delete("/endpoints/{id}", s.handleDeleteEndpoint)
				// Static segment wins over {id} in chi; registered first for clarity.
				r.Post("/endpoints/prune-dead", s.handlePruneDeadEndpoints)
				r.Post("/endpoints/{id}/probe", s.handleProbeEndpoint)

				r.Post("/discovery/run", s.handleRunDiscovery)

				// evals.create runs an assessment against a hub's models using
				// the global case library — it consumes cases, does not write
				// them, so it is a hub-scoped operation (spec 0005 decision 6:
				// the "global write = super_admin" list is settings/
				// classification_rules/suites/cases, not evals).
				r.Post("/evals", s.handleCreateEval)

				r.Post("/campaigns/{id}/share-links", s.handleCreateShareLink)
				r.Delete("/share-links/{id}", s.handleRevokeShareLink)
			})

			// Reads: any authenticated user (super_admin/admin/operator/viewer).
			// /api/users CRUD — ticket 67. GET is admin+super_admin; writes
			// (POST/PATCH/PUT/DELETE) are admin+super_admin and enforced
			// per-target by assertCanManageUser (cross-hub → 403).
			r.Group(func(r chi.Router) {
				r.Use(s.requireRole(store.RoleSuperAdmin, store.RoleAdmin))
				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Patch("/users/{id}", s.handlePatchUser)
				r.Put("/users/{id}/password", s.handleResetUserPassword)
				r.Delete("/users/{id}", s.handleDeleteUser)
			})

			r.Get("/hubs", s.handleListHubs)
			r.Get("/models", s.handleListModels)
			r.Get("/models/{id}/eval-summary", s.handleGetModelEvalSummary)

			// Static segment wins over {id} in chi; registered first for clarity.
			r.Get("/endpoints/{id}", s.handleGetEndpointDetail)
			r.Get("/endpoints/{id}/series", s.handleGetEndpointSeries)
			r.Get("/endpoints/{id}/probes", s.handleListProbes)

			r.Get("/classification-rules", s.handleListClassificationRules)

			r.Get("/overview", s.handleGetOverview)

			r.Get("/suites", s.handleListSuites)
			// Static segment wins over {id} in chi; registered first for clarity.
			r.Get("/evals/latest", s.handleLatestEvals)
			r.Get("/evals", s.handleListEvals)
			r.Get("/evals/{id}", s.handleGetEval)

			r.Get("/campaigns", s.handleListCampaigns)
			r.Get("/campaigns/{id}", s.handleGetCampaign)
			r.Get("/campaigns/{id}/report", s.handleGetCampaignReport)
			r.Get("/campaigns/{id}/trends", s.handleGetCampaignTrends)

			// Public eval board (spec 0010): anonymous like the status board
			// via the publicReadPattern whitelist; serves the newest settled
			// campaign's report at the same information level as the
			// token-gated shared report.
			r.Get("/public/eval/board", s.handleGetPublicEvalBoard)

			r.Get("/share-links", s.handleListShareLinks)
			// Public by token (ADR 0006): the requireSession middleware lets
			// this one GET path through without a session; the token itself
			// is the credential, and unknown/revoked tokens answer a uniform
			// 404. No other API is exposed by a share token.
			r.Get("/shared-reports/{token}", s.handleGetSharedReport)

			r.Get("/settings", s.handleGetSettings)
			r.Get("/alerts", s.handleListAlerts)

			r.Get("/tasks", s.handleListTasks)
			r.Get("/tasks/{id}", s.handleGetTask)

			// Static segment wins over {id} in chi; registered first for clarity.
			r.Get("/audit-logs/actions", s.handleListAuditActions)
			r.Get("/audit-logs", s.handleListAuditLogs)
		})
	})

	// Any unmatched route is handled by the SPA (or JSON 404 under /api).
	r.NotFound(s.serveSPA)

	return r
}

// writeData writes a success envelope {"data": ...} with the given status.
func writeData(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// writeError writes an error envelope {"error": {"message": ...}}.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"message": message},
	})
}

// writeNoContent writes a 204 with no body.
func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// handleVersion returns the server version string.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	version := s.version
	if version == "" {
		version = "unknown"
	}
	writeData(w, http.StatusOK, map[string]string{"version": version})
}
