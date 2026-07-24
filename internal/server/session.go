package server

import (
	"context"
	"net/http"

	"github.com/taliove/hubscope/internal/store"
)

// SessionUser carries the authenticated identity through the request context.
// It is injected by requireSession after the session cookie is validated, so
// downstream handlers and s.audit can read the role, hub scope, and username
// without re-querying the users table.
type SessionUser struct {
	UserID   int64
	Role     string
	HubID    *int64
	Username string
}

// sessionUserKey is an unexported context key type (Go convention to avoid
// collisions with other packages that might use a string key).
type sessionUserKey struct{}

// withSessionUser returns a new context carrying the given SessionUser.
func withSessionUser(ctx context.Context, u SessionUser) context.Context {
	return context.WithValue(ctx, sessionUserKey{}, u)
}

// sessionUser extracts the SessionUser from the request context. It returns
// nil when the request was not authenticated (e.g., a public-read bypass);
// callers must nil-check before dereferencing.
func sessionUser(r *http.Request) *SessionUser {
	if u, ok := r.Context().Value(sessionUserKey{}).(SessionUser); ok {
		return &u
	}
	return nil
}

// actorOr resolves the audit actor from the request context, falling back to
// "system" when no user is present. The fallback is defensive: every audit
// call site lives inside an HTTP handler behind requireSession, so a nil
// SessionUser should not occur in practice (background jobs do not write
// audit logs — verified at design time).
func actorOr(r *http.Request) string {
	if u := sessionUser(r); u != nil {
		return u.Username
	}
	return "system"
}

// requireRole returns a middleware that admits only the listed roles. It must
// be stacked after requireSession, which injects the SessionUser into the
// context; a request whose role is not in the allow-list answers 403
// ("insufficient role") — distinct from the 401 requireSession emits for an
// unauthenticated request, so the front end can tell "not logged in" from
// "logged in but not permitted".
func (s *Server) requireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := sessionUser(r)
			if u == nil {
				// No session injected: treat as forbidden rather than
				// unauthenticated, since requireSession should have already
				// gated public reads. This branch is defensive.
				writeError(w, http.StatusForbidden, "insufficient role")
				return
			}
			if _, ok := allowed[u.Role]; !ok {
				writeError(w, http.StatusForbidden, "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// roleAllow reports whether the session user's role is in the allow-list.
// Kept as a small helper for readability in route wiring; not exported.
func roleAllow(u *SessionUser, roles ...string) bool {
	if u == nil {
		return false
	}
	for _, r := range roles {
		if u.Role == r {
			return true
		}
	}
	return false
}

// Compile-time guard: ensure store role constants are referenced so renames
// surface here rather than at runtime.
var _ = []string{
	store.RoleSuperAdmin,
	store.RoleAdmin,
	store.RoleOperator,
	store.RoleViewer,
}
