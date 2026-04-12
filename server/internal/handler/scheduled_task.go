package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/scheduler"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ScheduledTaskResponse is the wire shape for a scheduled_task row.
type ScheduledTaskResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	AgentID     string  `json:"agent_id"`
	CreatedBy   string  `json:"created_by"`
	Title       string  `json:"title"`
	Prompt      string  `json:"prompt"`
	CronExpr    string  `json:"cron_expr"`
	Enabled     bool    `json:"enabled"`
	LastRunAt   *string `json:"last_run_at"`
	NextRunAt   string  `json:"next_run_at"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func scheduledTaskToResponse(s db.ScheduledTask) ScheduledTaskResponse {
	return ScheduledTaskResponse{
		ID:          uuidToString(s.ID),
		WorkspaceID: uuidToString(s.WorkspaceID),
		AgentID:     uuidToString(s.AgentID),
		CreatedBy:   uuidToString(s.CreatedBy),
		Title:       s.Title,
		Prompt:      s.Prompt,
		CronExpr:    s.CronExpr,
		Enabled:     s.Enabled,
		LastRunAt:   timestampToPtr(s.LastRunAt),
		NextRunAt:   timestampToString(s.NextRunAt),
		CreatedAt:   timestampToString(s.CreatedAt),
		UpdatedAt:   timestampToString(s.UpdatedAt),
	}
}

// ListScheduledTasks returns all schedules for the current workspace.
func (h *Handler) ListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	workspaceID := resolveWorkspaceID(r)
	rows, err := h.Queries.ListScheduledTasks(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list scheduled tasks")
		return
	}
	resp := make([]ScheduledTaskResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, scheduledTaskToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

type CreateScheduledTaskRequest struct {
	AgentID  string `json:"agent_id"`
	Title    string `json:"title"`
	Prompt   string `json:"prompt"`
	CronExpr string `json:"cron_expr"`
}

// CreateScheduledTask validates and persists a new schedule. The cron
// expression is evaluated once up-front so we can reject invalid input
// and seed next_run_at with the real next fire time.
func (h *Handler) CreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	workspaceID := resolveWorkspaceID(r)
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req CreateScheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" || req.Title == "" || req.Prompt == "" || req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "agent_id, title, prompt, cron_expr are required")
		return
	}

	// Verify agent belongs to the workspace.
	agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          parseUUID(req.AgentID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "agent not found in workspace")
		return
	}

	next, err := scheduler.ComputeNextRun(req.CronExpr, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron_expr: "+err.Error())
		return
	}

	created, err := h.Queries.CreateScheduledTask(r.Context(), db.CreateScheduledTaskParams{
		WorkspaceID: parseUUID(workspaceID),
		AgentID:     agent.ID,
		CreatedBy:   parseUUID(userID),
		Title:       req.Title,
		Prompt:      req.Prompt,
		CronExpr:    req.CronExpr,
		NextRunAt:   pgtype.Timestamptz{Time: next, Valid: true},
	})
	if err != nil {
		slog.Warn("create scheduled task failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create scheduled task")
		return
	}

	writeJSON(w, http.StatusCreated, scheduledTaskToResponse(created))
}

type UpdateScheduledTaskRequest struct {
	AgentID  *string `json:"agent_id"`
	Title    *string `json:"title"`
	Prompt   *string `json:"prompt"`
	CronExpr *string `json:"cron_expr"`
	Enabled  *bool   `json:"enabled"`
}

// UpdateScheduledTask applies partial updates. When cron_expr changes
// we recompute next_run_at so the schedule immediately respects the
// new cadence.
func (h *Handler) UpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	workspaceID := resolveWorkspaceID(r)
	id := chi.URLParam(r, "id")

	existing, err := h.Queries.GetScheduledTaskInWorkspace(r.Context(), db.GetScheduledTaskInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	var req UpdateScheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateScheduledTaskParams{ID: existing.ID}
	if req.Title != nil {
		params.Title = pgtype.Text{String: *req.Title, Valid: true}
	}
	if req.Prompt != nil {
		params.Prompt = pgtype.Text{String: *req.Prompt, Valid: true}
	}
	if req.AgentID != nil {
		agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
			ID:          parseUUID(*req.AgentID),
			WorkspaceID: parseUUID(workspaceID),
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "agent not found in workspace")
			return
		}
		params.AgentID = agent.ID
	}
	if req.Enabled != nil {
		params.Enabled = pgtype.Bool{Bool: *req.Enabled, Valid: true}
	}
	if req.CronExpr != nil {
		next, err := scheduler.ComputeNextRun(*req.CronExpr, time.Now())
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cron_expr: "+err.Error())
			return
		}
		params.CronExpr = pgtype.Text{String: *req.CronExpr, Valid: true}
		params.NextRunAt = pgtype.Timestamptz{Time: next, Valid: true}
	}

	updated, err := h.Queries.UpdateScheduledTask(r.Context(), params)
	if err != nil {
		slog.Warn("update scheduled task failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to update scheduled task")
		return
	}
	writeJSON(w, http.StatusOK, scheduledTaskToResponse(updated))
}

// DeleteScheduledTask removes a schedule permanently.
func (h *Handler) DeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	workspaceID := resolveWorkspaceID(r)
	id := chi.URLParam(r, "id")

	if _, err := h.Queries.GetScheduledTaskInWorkspace(r.Context(), db.GetScheduledTaskInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	}); err != nil {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	}

	if err := h.Queries.DeleteScheduledTask(r.Context(), parseUUID(id)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete scheduled task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
