package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/prober"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// Server holds dependencies and the HTTP router.
type Server struct {
	db       *store.DB
	prober   *prober.Prober
	router   chi.Router
	staticFS fs.FS

	// adminPassword is kept in memory only for login comparison; it is never
	// logged, persisted, or returned in any response. sessionKey is derived
	// from it and signs the stateless session cookie.
	adminPassword string
	sessionKey    []byte
}

// New builds a Server with all API routes registered. The admin password is
// required: it backs both login comparison and session cookie signing.
func New(db *store.DB, adminPassword string) *Server {
	if adminPassword == "" {
		panic("server.New: admin password must not be empty")
	}
	s := &Server{
		db:            db,
		prober:        prober.New(db, hubclient.New()),
		adminPassword: adminPassword,
		sessionKey:    deriveSessionKey(adminPassword),
	}
	s.router = s.routes()
	return s
}

// ServeHTTP makes Server an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// routes wires up all API endpoints plus the health check.
func (s *Server) routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	r.Route("/api", func(r chi.Router) {
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

			r.Post("/models", s.handleCreateModel)
			r.Get("/models", s.handleListModels)

			r.Patch("/endpoints/{id}", s.handlePatchEndpoint)
			r.Post("/endpoints/{id}/probe", s.handleProbeEndpoint)
			r.Get("/endpoints/{id}/probes", s.handleListProbes)
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
