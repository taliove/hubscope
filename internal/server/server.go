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

	// adminPassword is kept in memory only for login comparison; it is never
	// logged, persisted, or returned in any response. sessionKey is derived
	// from it and signs the stateless session cookie.
	adminPassword string
	sessionKey    []byte
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

// New builds a Server with all API routes registered. The admin password is
// required: it backs both login comparison and session cookie signing.
func New(db *store.DB, adminPassword string, opts ...Option) *Server {
	if adminPassword == "" {
		panic("server.New: admin password must not be empty")
	}
	limits := defaultRateLimits()
	s := &Server{
		db:            db,
		prober:        prober.New(db, hubclient.New()),
		discovery:     discovery.New(db, hubclient.New()),
		evaluator:     evaluator.New(db, hubclient.NewWithTimeout(evaluator.RequestTimeout)),
		alerter:       alerter.NewEvaluator(db, alerter.NewLarkSender()),
		adminPassword: adminPassword,
		sessionKey:    deriveSessionKey(adminPassword),
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

			r.Post("/hubs", s.handleCreateHub)
			r.Get("/hubs", s.handleListHubs)
			r.Put("/hubs/{id}", s.handleUpdateHub)
			r.Delete("/hubs/{id}", s.handleDeleteHub)
			r.Post("/hubs/{id}/sync", s.handleSyncHub)

			r.Post("/models", s.handleCreateModel)
			r.Get("/models", s.handleListModels)
			r.Delete("/models/{id}", s.handleDeleteModel)
			r.Post("/models/{id}/trial", s.handleTrialModel)

			r.Patch("/endpoints/{id}", s.handlePatchEndpoint)
			r.Delete("/endpoints/{id}", s.handleDeleteEndpoint)
			// Static segment wins over {id} in chi; registered first for clarity.
			r.Post("/endpoints/prune-dead", s.handlePruneDeadEndpoints)
			r.Post("/endpoints/{id}/probe", s.handleProbeEndpoint)
			r.Get("/endpoints/{id}", s.handleGetEndpointDetail)
			r.Get("/endpoints/{id}/series", s.handleGetEndpointSeries)
			r.Get("/endpoints/{id}/probes", s.handleListProbes)

			r.Post("/discovery/run", s.handleRunDiscovery)

			r.Get("/classification-rules", s.handleListClassificationRules)
			r.Post("/classification-rules", s.handleCreateClassificationRule)
			r.Patch("/classification-rules/{id}", s.handlePatchClassificationRule)
			r.Delete("/classification-rules/{id}", s.handleDeleteClassificationRule)

			r.Get("/overview", s.handleGetOverview)

			r.Get("/suites", s.handleListSuites)
			r.Post("/cases", s.handleCreateCase)
			r.Patch("/cases/{id}", s.handlePatchCase)
			r.Post("/evals", s.handleCreateEval)
			r.Get("/evals", s.handleListEvals)
			// Static segment wins over {id} in chi; registered first for clarity.
			r.Get("/evals/latest", s.handleLatestEvals)
			r.Get("/evals/{id}", s.handleGetEval)

			r.Get("/campaigns", s.handleListCampaigns)
			r.Get("/campaigns/{id}", s.handleGetCampaign)
			r.Get("/campaigns/{id}/report", s.handleGetCampaignReport)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
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
