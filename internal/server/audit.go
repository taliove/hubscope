package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"git.github.net/taliove2009/ai-hub-checker/internal/store"
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
}

// auditPageResponse is the payload for GET /api/audit-logs.
type auditPageResponse struct {
	Items    []auditLogDTO `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// auditActor is the actor recorded for every authenticated write; the system
// has a single admin identity.
const auditActor = "admin"

// audit records one administrative action. Audit failures are logged but
// never fail the request itself.
func (s *Server) audit(r *http.Request, action, objectType, objectID, detail, result string) {
	err := s.db.InsertAudit(store.AuditLog{
		Actor:      auditActor,
		IP:         s.clientIP(r),
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Detail:     detail,
		Result:     result,
	})
	if err != nil {
		slog.Error("audit: insert failed", "action", action, "error", err)
	}
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
	}
}

// handleListAuditLogs handles GET /api/audit-logs?page=N&page_size=M&action=A.
func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parsePositiveInt(q.Get("page"), 1)
	pageSize := parsePositiveInt(q.Get("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}

	logs, total, err := s.db.ListAuditLogs(page, pageSize, q.Get("action"))
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
