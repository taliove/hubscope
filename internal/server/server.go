package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.github.net/taliove2009/ai-hub-checker/internal/evaluator"
	"git.github.net/taliove2009/ai-hub-checker/internal/hubclient"
	"git.github.net/taliove2009/ai-hub-checker/internal/prober"
	"git.github.net/taliove2009/ai-hub-checker/internal/store"
)

// Server holds dependencies and the HTTP router.
type Server struct {
	db        *store.DB
	prober    *prober.Prober
	evaluator *evaluator.Evaluator
	router    chi.Router
	staticFS  fs.FS
}

// New builds a Server with all API routes registered.
func New(db *store.DB) *Server {
	s := &Server{
		db:        db,
		prober:    prober.New(db, hubclient.New()),
		evaluator: evaluator.New(db, hubclient.NewWithTimeout(evaluator.RequestTimeout)),
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
		r.Post("/hubs", s.handleCreateHub)
		r.Get("/hubs", s.handleListHubs)
		r.Put("/hubs/{id}", s.handleUpdateHub)
		r.Delete("/hubs/{id}", s.handleDeleteHub)

		r.Post("/models", s.handleCreateModel)
		r.Get("/models", s.handleListModels)

		r.Patch("/endpoints/{id}", s.handlePatchEndpoint)
		r.Post("/endpoints/{id}/probe", s.handleProbeEndpoint)
		r.Get("/endpoints/{id}/probes", s.handleListProbes)

		r.Get("/suites", s.handleListSuites)
		r.Post("/cases", s.handleCreateCase)
		r.Patch("/cases/{id}", s.handlePatchCase)
		r.Post("/evals", s.handleCreateEval)
		r.Get("/evals", s.handleListEvals)
		r.Get("/evals/{id}", s.handleGetEval)
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
