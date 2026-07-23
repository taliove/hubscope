package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/taliove2009/hubscope/internal/store"
)

// auditLogDTO is the API representation of one audit entry.
type auditLogDTO struct {
	ID         int64  `json:"id"`
	At         string `json:"at"`
	Actor      string `json:"actor"`
	IP         string `json:"ip"`
	Action     string `json:"action"`
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Detail     string `json:"detail"`
	Result     string `json:"result"`
	HubID      *int64 `json:"hub_id"`
}

// auditPageResponse is the payload for GET /api/audit-logs.
type auditPageResponse struct {
	Items    []auditLogDTO `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// audit records one administrative action. The actor is the logged-in
// username, read from the request context (injected by requireSession); the
// "system" fallback applies only when no user is present, which in practice
// never happens since every call site is an HTTP handler behind
// requireSession (background jobs do not write audit logs). The hub_id is
// likewise read from the context (hubIDOr): a hub-scoped admin stamps the
// row with their own hub so it is visible only within that hub, while a
// super_admin (and the hub-less auth.login user-not-found branch) writes
// NULL, which only super_admin can read. Audit failures are logged but
// never fail the request itself.
func (s *Server) audit(r *http.Request, action, objectType, objectID, detail, result string) {
	err := s.db.InsertAudit(store.AuditLog{
		Actor:      actorOr(r),
		IP:         s.clientIP(r),
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Detail:     detail,
		Result:     result,
		HubID:      hubIDOr(r),
	})
	if err != nil {
		slog.Error("audit: insert failed", "action", action, "error", err)
	}
}

// hubIDOr resolves the actor's hub_id from the request context, returning nil
// when no session user is present (public-read bypass or the auth.login
// user-not-found branch, which has no user to inject). nil means "no single
// hub" — the row is super_admin-only.
func hubIDOr(r *http.Request) *int64 {
	if u := sessionUser(r); u != nil {
		return u.HubID
	}
	return nil
}

// toAuditDTO maps a store.AuditLog to its API representation.
func toAuditDTO(l store.AuditLog) auditLogDTO {
	return auditLogDTO{
		ID:         l.ID,
		At:         l.At.Format(time.RFC3339),
		Actor:      l.Actor,
		IP:         l.IP,
		Action:     l.Action,
		ObjectType: l.ObjectType,
		ObjectID:   l.ObjectID,
		Detail:     l.Detail,
		Result:     l.Result,
		HubID:      l.HubID,
	}
}

// handleListAuditLogs handles GET /api/audit-logs?page=N&page_size=M&action=A.
// A super_admin sees every row (including NULL-hub rows); a hub-scoped admin
// sees only rows stamped with their own hub_id (spec 0005 per-hub isolation,
// extended to the audit log by ticket 66).
func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parsePositiveInt(q.Get("page"), 1)
	pageSize := parsePositiveInt(q.Get("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}
	action := q.Get("action")

	var (
		logs  []store.AuditLog
		total int
		err   error
	)
	u := sessionUser(r)
	if u != nil && u.Role == store.RoleSuperAdmin {
		logs, total, err = s.db.ListAuditLogsAll(page, pageSize, action)
	} else if u != nil && u.HubID != nil {
		logs, total, err = s.db.ListAuditLogsByHub(*u.HubID, page, pageSize, action)
	} else {
		// No hub scope and not super_admin: a defensive branch (a disabled
		// user is rejected at the gate; a viewer without a hub would see an
		// empty log rather than another hub's rows).
		logs, total, err = s.db.ListAuditLogsByHub(0, page, pageSize, action)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit logs")
		return
	}

	items := make([]auditLogDTO, 0, len(logs))
	for _, l := range logs {
		items = append(items, toAuditDTO(l))
	}
	writeData(w, http.StatusOK, auditPageResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// handleListAuditActions handles GET /api/audit-logs/actions, feeding the
// action filter dropdown.
func (s *Server) handleListAuditActions(w http.ResponseWriter, r *http.Request) {
	actions, err := s.db.ListAuditActions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit actions")
		return
	}
	if actions == nil {
		actions = []string{}
	}
	writeData(w, http.StatusOK, actions)
}

// parsePositiveInt parses a positive integer query value, falling back to def.
func parsePositiveInt(raw string, def int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
