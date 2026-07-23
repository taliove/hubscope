package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/taliove2009/hubscope/internal/alerter"
	"github.com/taliove2009/hubscope/internal/discovery"
	"github.com/taliove2009/hubscope/internal/evaluator"
	"github.com/taliove2009/hubscope/internal/hubclient"
	"github.com/taliove2009/hubscope/internal/prober"
	"github.com/taliove2009/hubscope/internal/store"
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

	// Rate-limit tiers; a nil limiter means its tier is unlimited.
	loginLimiter *ipLimiter
	writeLimiter *ipLimiter
	readLimiter  *ipLimiter
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
	}
}

// WithTrustProxy controls whether X-Forwarded-For is trusted when resolving
// client IPs. Enable only behind a forwarding proxy that sets the header.
func WithTrustProxy(trust bool) Option {
	return func(s *Server) {
		s.trustProxy = trust
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
	}
	// Alert evaluation hooks into every probe round served by this prober.
	// main hooks the scheduler's prober into the same evaluator via Alerter().
	s.prober.AfterRound = s.alerter.HandleRound
	// Score-drop checks hook into every settled eval campaign served by this
	// evaluator; the weekly eval worker shares it via Evaluator().
	s.evaluator.AfterCampaign = s.alerter.HandleCampaign
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

	r.Route("/api", func(r chi.Router) {
		r.Use(s.rateLimit)
		r.Use(limitBodySize)
		// Auth endpoints stay open; login issues the session cookie.
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleAuthMe)
		})

		// All other API routes: GET is public, writes require a session.
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
