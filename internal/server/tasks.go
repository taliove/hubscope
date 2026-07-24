package server

import (
	"net/http"
	"time"

	"github.com/taliove/hubscope/internal/store"
)

// taskDTO is the API representation of a Task. duration_ms is the wall-clock
// execution time in milliseconds, null until the task reaches a terminal
// state (or when it never started).
type taskDTO struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Source     string  `json:"source"`
	Status     string  `json:"status"`
	EntityType string  `json:"entity_type"`
	EntityID   int64   `json:"entity_id"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	DurationMs *int64  `json:"duration_ms"`
	CreatedAt  string  `json:"created_at"`
}

// taskLogDTO is the API representation of one task log line.
type taskLogDTO struct {
	ID      int64  `json:"id"`
	At      string `json:"at"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// taskPageResponse is the payload for GET /api/tasks.
type taskPageResponse struct {
	Items    []taskDTO `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// taskDetailDTO is a task plus its full execution log.
type taskDetailDTO struct {
	taskDTO
	Logs []taskLogDTO `json:"logs"`
}

// toTaskDTO maps a store.Task to its API representation.
func toTaskDTO(t store.Task) taskDTO {
	dto := taskDTO{
		ID:         t.ID,
		Type:       t.Type,
		Source:     t.Source,
		Status:     t.Status,
		EntityType: t.EntityType,
		EntityID:   t.EntityID,
		CreatedAt:  t.CreatedAt.Format(time.RFC3339),
	}
	if t.StartedAt != nil {
		s := t.StartedAt.Format(time.RFC3339)
		dto.StartedAt = &s
	}
	if t.FinishedAt != nil {
		s := t.FinishedAt.Format(time.RFC3339)
		dto.FinishedAt = &s
	}
	if t.StartedAt != nil && t.FinishedAt != nil {
		ms := t.FinishedAt.Sub(*t.StartedAt).Milliseconds()
		dto.DurationMs = &ms
	}
	return dto
}

// toTaskLogDTO maps a store.TaskLog to its API representation.
func toTaskLogDTO(l store.TaskLog) taskLogDTO {
	return taskLogDTO{
		ID:      l.ID,
		At:      l.At.Format(time.RFC3339),
		Level:   l.Level,
		Message: l.Message,
	}
}

// handleListTasks handles GET /api/tasks?type=T&status=S&page=N&page_size=M.
// Tasks are monitoring data: reads follow the same session-gated tier as
// eval runs and audit logs, scoped to the session's hub for non-super_admin.
// Hub-less tasks (rollup/retention) are super_admin-only via the *All store
// variant.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parsePositiveInt(q.Get("page"), 1)
	pageSize := parsePositiveInt(q.Get("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}

	u := sessionUser(r)
	var tasks []store.Task
	var total int
	var err error
	if u == nil || u.Role == store.RoleSuperAdmin {
		tasks, total, err = s.db.ListTasksAll(page, pageSize, q.Get("type"), q.Get("status"))
	} else if u.HubID == nil {
		tasks, total, err = []store.Task{}, 0, error(nil)
	} else {
		tasks, total, err = s.db.ListTasksByHub(*u.HubID, page, pageSize, q.Get("type"), q.Get("status"))
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	items := make([]taskDTO, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toTaskDTO(t))
	}
	writeData(w, http.StatusOK, taskPageResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// handleGetTask handles GET /api/tasks/{id}, including the execution log.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	task, err := s.db.GetTask(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	logs, err := s.db.ListTaskLogs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load task logs")
		return
	}

	logDTOs := make([]taskLogDTO, 0, len(logs))
	for _, l := range logs {
		logDTOs = append(logDTOs, toTaskLogDTO(l))
	}
	writeData(w, http.StatusOK, taskDetailDTO{
		taskDTO: toTaskDTO(*task),
		Logs:    logDTOs,
	})
}
