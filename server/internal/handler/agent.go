package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// validateSupervisor checks that a candidate supervisor agent exists, is in
// the given workspace, and is not archived. Used when creating/updating an
// agent's reports_to link. Returns the validated supervisor record so the
// caller can reuse it without an extra DB roundtrip.
//
// NOTE: this does NOT check for cycles — that's the caller's job on updates,
// and is impossible on creates (a brand-new agent has no descendants yet).
func (h *Handler) validateSupervisor(r *http.Request, supervisorID string, workspaceID pgtype.UUID) (db.Agent, error) {
	sup, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          parseUUID(supervisorID),
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return db.Agent{}, fmt.Errorf("supervisor not found in workspace")
	}
	if sup.ArchivedAt.Valid {
		return db.Agent{}, fmt.Errorf("supervisor is archived")
	}
	return sup, nil
}

// checkNoSupervisorCycle guards against creating a loop in the reports_to
// graph. It rejects the change if the proposed supervisor is a transitive
// descendant of the agent being updated (e.g. A -> B -> C, then trying to
// set A.reports_to = C would make C -> A -> B -> C).
func (h *Handler) checkNoSupervisorCycle(r *http.Request, agentID, supervisorID pgtype.UUID) error {
	if !agentID.Valid || !supervisorID.Valid {
		return nil
	}
	descendants, err := h.Queries.ListAgentDescendants(r.Context(), agentID)
	if err != nil {
		return fmt.Errorf("failed to check supervisor cycle: %w", err)
	}
	for _, d := range descendants {
		if d.Bytes == supervisorID.Bytes {
			return fmt.Errorf("cannot set supervisor: would create a cycle in the agent hierarchy")
		}
	}
	return nil
}

// validateRuntimeConfig enforces the runtime_config schema. Returns a non-nil
// error if the payload is malformed. Note: the path is NOT checked for
// existence here because runtime_config.workdir_path refers to the daemon's
// host filesystem, which may be a different machine than the server. The
// daemon validates existence at task execution time.
//
// Schema recognized today:
//   workdir_mode: "" | "isolated" | "direct"
//   workdir_path: absolute host path, required when workdir_mode == "direct"
//
// Unknown keys are ignored so future knobs can be added without breaking
// existing clients.
func validateRuntimeConfig(raw any) error {
	if raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("runtime_config must be a JSON object")
	}
	modeVal, hasMode := m["workdir_mode"]
	pathVal, hasPath := m["workdir_path"]

	var mode, path string
	if hasMode {
		mode, ok = modeVal.(string)
		if !ok {
			return fmt.Errorf("workdir_mode must be a string")
		}
	}
	if hasPath {
		path, ok = pathVal.(string)
		if !ok {
			return fmt.Errorf("workdir_path must be a string")
		}
	}

	switch mode {
	case "", "isolated":
		// Default mode — no further checks.
		return nil
	case "direct":
		if path == "" {
			return fmt.Errorf("workdir_path is required when workdir_mode is \"direct\"")
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("workdir_path must be an absolute path")
		}
		return nil
	default:
		return fmt.Errorf("workdir_mode %q is not supported (use \"isolated\" or \"direct\")", mode)
	}
}

type AgentResponse struct {
	ID                 string          `json:"id"`
	WorkspaceID        string          `json:"workspace_id"`
	RuntimeID          string          `json:"runtime_id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Instructions       string          `json:"instructions"`
	AvatarURL          *string         `json:"avatar_url"`
	RuntimeMode        string          `json:"runtime_mode"`
	RuntimeConfig      any             `json:"runtime_config"`
	Visibility         string          `json:"visibility"`
	Status             string          `json:"status"`
	MaxConcurrentTasks int32           `json:"max_concurrent_tasks"`
	OwnerID            *string         `json:"owner_id"`
	ReportsTo          *string         `json:"reports_to"`
	// Budget: NULL means unlimited. Cents for precision, UI converts to $.
	BudgetMonthlyCents *int64          `json:"budget_monthly_cents"`
	SpentMonthlyCents  int64           `json:"spent_monthly_cents"`
	BudgetPeriodStart  string          `json:"budget_period_start"`
	Skills             []SkillResponse `json:"skills"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
	ArchivedAt         *string         `json:"archived_at"`
	ArchivedBy         *string         `json:"archived_by"`
}

func agentToResponse(a db.Agent) AgentResponse {
	var rc any
	if a.RuntimeConfig != nil {
		json.Unmarshal(a.RuntimeConfig, &rc)
	}
	if rc == nil {
		rc = map[string]any{}
	}

	var budget *int64
	if a.BudgetMonthlyCents.Valid {
		v := a.BudgetMonthlyCents.Int64
		budget = &v
	}
	var periodStart string
	if a.BudgetPeriodStart.Valid {
		periodStart = a.BudgetPeriodStart.Time.Format("2006-01-02")
	}

	return AgentResponse{
		ID:                 uuidToString(a.ID),
		WorkspaceID:        uuidToString(a.WorkspaceID),
		RuntimeID:          uuidToString(a.RuntimeID),
		Name:               a.Name,
		Description:        a.Description,
		Instructions:       a.Instructions,
		AvatarURL:          textToPtr(a.AvatarUrl),
		RuntimeMode:        a.RuntimeMode,
		RuntimeConfig:      rc,
		Visibility:         a.Visibility,
		Status:             a.Status,
		MaxConcurrentTasks: a.MaxConcurrentTasks,
		OwnerID:            uuidToPtr(a.OwnerID),
		ReportsTo:          uuidToPtr(a.ReportsTo),
		BudgetMonthlyCents: budget,
		SpentMonthlyCents:  a.SpentMonthlyCents,
		BudgetPeriodStart:  periodStart,
		Skills:             []SkillResponse{},
		CreatedAt:          timestampToString(a.CreatedAt),
		UpdatedAt:          timestampToString(a.UpdatedAt),
		ArchivedAt:         timestampToPtr(a.ArchivedAt),
		ArchivedBy:         uuidToPtr(a.ArchivedBy),
	}
}

// RepoData holds repository information included in claim responses so the
// daemon can set up worktrees for each workspace repo.
type RepoData struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type AgentTaskResponse struct {
	ID             string         `json:"id"`
	AgentID        string         `json:"agent_id"`
	RuntimeID      string         `json:"runtime_id"`
	IssueID        string         `json:"issue_id"`
	WorkspaceID    string         `json:"workspace_id"`
	Status         string         `json:"status"`
	Priority       int32          `json:"priority"`
	DispatchedAt   *string        `json:"dispatched_at"`
	StartedAt      *string        `json:"started_at"`
	CompletedAt    *string        `json:"completed_at"`
	Result         any            `json:"result"`
	Error          *string        `json:"error"`
	Agent          *TaskAgentData `json:"agent,omitempty"`
	Repos          []RepoData     `json:"repos,omitempty"`
	CreatedAt      string         `json:"created_at"`
	PriorSessionID   string         `json:"prior_session_id,omitempty"`    // session ID from a previous task on same issue
	PriorWorkDir     string         `json:"prior_work_dir,omitempty"`     // work_dir from a previous task on same issue
	TriggerCommentID *string        `json:"trigger_comment_id,omitempty"` // comment that triggered this task
	ChatSessionID    string         `json:"chat_session_id,omitempty"`    // non-empty for chat tasks
	ChatMessage      string         `json:"chat_message,omitempty"`       // user message for chat tasks
}

// TaskAgentData holds agent info included in claim responses so the daemon
// can set up the execution environment (branch naming, skill files, instructions).
type TaskAgentData struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Instructions  string                   `json:"instructions"`
	Skills        []service.AgentSkillData `json:"skills,omitempty"`
	// RuntimeConfig is the raw JSONB payload from the agents.runtime_config
	// column, passed through to the daemon so it can honor per-agent runtime
	// knobs (e.g. workdir_mode = "direct" to edit real host files).
	RuntimeConfig json.RawMessage `json:"runtime_config,omitempty"`
}

func taskToResponse(t db.AgentTaskQueue) AgentTaskResponse {
	var result any
	if t.Result != nil {
		json.Unmarshal(t.Result, &result)
	}
	return AgentTaskResponse{
		ID:           uuidToString(t.ID),
		AgentID:      uuidToString(t.AgentID),
		RuntimeID:    uuidToString(t.RuntimeID),
		IssueID:      uuidToString(t.IssueID),
		Status:       t.Status,
		Priority:     t.Priority,
		DispatchedAt: timestampToPtr(t.DispatchedAt),
		StartedAt:    timestampToPtr(t.StartedAt),
		CompletedAt:  timestampToPtr(t.CompletedAt),
		Result:       result,
		Error:            textToPtr(t.Error),
		CreatedAt:        timestampToString(t.CreatedAt),
		TriggerCommentID: uuidToPtr(t.TriggerCommentID),
	}
}

func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	workspaceID := resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}

	var agents []db.Agent
	var err error
	if r.URL.Query().Get("include_archived") == "true" {
		agents, err = h.Queries.ListAllAgents(r.Context(), parseUUID(workspaceID))
	} else {
		agents, err = h.Queries.ListAgents(r.Context(), parseUUID(workspaceID))
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	// Batch-load skills for all agents to avoid N+1.
	skillRows, err := h.Queries.ListAgentSkillsByWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load agent skills")
		return
	}
	skillMap := map[string][]SkillResponse{}
	for _, row := range skillRows {
		agentID := uuidToString(row.AgentID)
		skillMap[agentID] = append(skillMap[agentID], SkillResponse{
			ID:          uuidToString(row.ID),
			Name:        row.Name,
			Description: row.Description,
		})
	}

	// All agents (including private) are visible to workspace members.
	visible := make([]AgentResponse, 0, len(agents))
	for _, a := range agents {
		resp := agentToResponse(a)
		if skills, ok := skillMap[resp.ID]; ok {
			resp.Skills = skills
		}
		visible = append(visible, resp)
	}

	writeJSON(w, http.StatusOK, visible)
}

func (h *Handler) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, ok := h.loadAgentForUser(w, r, id)
	if !ok {
		return
	}
	resp := agentToResponse(agent)
	skills, err := h.Queries.ListAgentSkills(r.Context(), agent.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load agent skills")
		return
	}
	if len(skills) > 0 {
		resp.Skills = make([]SkillResponse, len(skills))
		for i, s := range skills {
			resp.Skills[i] = skillToResponse(s)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type CreateAgentRequest struct {
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	Instructions       string  `json:"instructions"`
	AvatarURL          *string `json:"avatar_url"`
	RuntimeID          string  `json:"runtime_id"`
	RuntimeConfig      any     `json:"runtime_config"`
	Visibility         string  `json:"visibility"`
	MaxConcurrentTasks int32   `json:"max_concurrent_tasks"`
	ReportsTo          *string `json:"reports_to"`
	BudgetMonthlyCents *int64  `json:"budget_monthly_cents"`
}

func (h *Handler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	workspaceID := resolveWorkspaceID(r)

	var req CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ownerID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.RuntimeID == "" {
		writeError(w, http.StatusBadRequest, "runtime_id is required")
		return
	}
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	if req.MaxConcurrentTasks == 0 {
		req.MaxConcurrentTasks = 6
	}

	runtime, err := h.Queries.GetAgentRuntimeForWorkspace(r.Context(), db.GetAgentRuntimeForWorkspaceParams{
		ID:          parseUUID(req.RuntimeID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid runtime_id")
		return
	}

	if err := validateRuntimeConfig(req.RuntimeConfig); err != nil {
		writeError(w, http.StatusBadRequest, "runtime_config: "+err.Error())
		return
	}

	rc, _ := json.Marshal(req.RuntimeConfig)
	if req.RuntimeConfig == nil {
		rc = []byte("{}")
	}

	// Optional: validate supervisor reference. Cycle check is skipped here
	// because a fresh agent has no descendants yet.
	var reportsTo pgtype.UUID
	if req.ReportsTo != nil && *req.ReportsTo != "" {
		if _, err := h.validateSupervisor(r, *req.ReportsTo, parseUUID(workspaceID)); err != nil {
			writeError(w, http.StatusBadRequest, "reports_to: "+err.Error())
			return
		}
		reportsTo = parseUUID(*req.ReportsTo)
	}

	agent, err := h.Queries.CreateAgent(r.Context(), db.CreateAgentParams{
		WorkspaceID:        parseUUID(workspaceID),
		Name:               req.Name,
		Description:        req.Description,
		Instructions:       req.Instructions,
		AvatarUrl:          ptrToText(req.AvatarURL),
		RuntimeMode:        runtime.RuntimeMode,
		RuntimeConfig:      rc,
		RuntimeID:          runtime.ID,
		Visibility:         req.Visibility,
		MaxConcurrentTasks: req.MaxConcurrentTasks,
		OwnerID:            parseUUID(ownerID),
		ReportsTo:          reportsTo,
	})
	if err != nil {
		slog.Warn("create agent failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to create agent: "+err.Error())
		return
	}
	slog.Info("agent created", append(logger.RequestAttrs(r), "agent_id", uuidToString(agent.ID), "name", agent.Name, "workspace_id", workspaceID)...)

	// Apply optional budget (create's base INSERT doesn't set it).
	if req.BudgetMonthlyCents != nil && *req.BudgetMonthlyCents >= 0 {
		bc := pgtype.Int8{Int64: *req.BudgetMonthlyCents, Valid: true}
		if updated, err := h.Queries.SetAgentBudget(r.Context(), db.SetAgentBudgetParams{
			ID:          agent.ID,
			BudgetCents: bc,
		}); err == nil {
			agent = updated
		}
	}

	if runtime.Status == "online" {
		h.TaskService.ReconcileAgentStatus(r.Context(), agent.ID)
		agent, _ = h.Queries.GetAgent(r.Context(), agent.ID)
	}

	resp := agentToResponse(agent)
	actorType, actorID := h.resolveActor(r, ownerID, workspaceID)
	h.publish(protocol.EventAgentCreated, workspaceID, actorType, actorID, map[string]any{"agent": resp})
	writeJSON(w, http.StatusCreated, resp)
}



type UpdateAgentRequest struct {
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	Instructions       *string `json:"instructions"`
	AvatarURL          *string `json:"avatar_url"`
	RuntimeID          *string `json:"runtime_id"`
	RuntimeConfig      any     `json:"runtime_config"`
	Visibility         *string `json:"visibility"`
	Status             *string `json:"status"`
	MaxConcurrentTasks *int32  `json:"max_concurrent_tasks"`
	// ReportsTo uses raw JSON so we can distinguish:
	//   absent             -> leave supervisor unchanged
	//   explicit null      -> clear supervisor
	//   "uuid-string"      -> set supervisor
	ReportsTo json.RawMessage `json:"reports_to,omitempty"`
	// BudgetMonthlyCents follows the same three-state pattern:
	//   absent  -> leave budget unchanged
	//   null    -> clear budget (unlimited)
	//   number  -> set budget to this cents value
	BudgetMonthlyCents json.RawMessage `json:"budget_monthly_cents,omitempty"`
}

// canManageAgent checks whether the current user can update or archive an agent.
// Only the agent owner or workspace owner/admin can manage any agent,
// regardless of whether it is public or private.
func (h *Handler) canManageAgent(w http.ResponseWriter, r *http.Request, agent db.Agent) bool {
	wsID := uuidToString(agent.WorkspaceID)
	member, ok := h.requireWorkspaceRole(w, r, wsID, "agent not found", "owner", "admin", "member")
	if !ok {
		return false
	}
	isAdmin := roleAllowed(member.Role, "owner", "admin")
	isAgentOwner := uuidToString(agent.OwnerID) == requestUserID(r)
	if !isAdmin && !isAgentOwner {
		writeError(w, http.StatusForbidden, "only the agent owner can manage this agent")
		return false
	}
	return true
}

func (h *Handler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, ok := h.loadAgentForUser(w, r, id)
	if !ok {
		return
	}
	if !h.canManageAgent(w, r, agent) {
		return
	}

	var req UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateAgentParams{
		ID: parseUUID(id),
	}
	if req.Name != nil {
		params.Name = pgtype.Text{String: *req.Name, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Instructions != nil {
		params.Instructions = pgtype.Text{String: *req.Instructions, Valid: true}
	}
	if req.AvatarURL != nil {
		params.AvatarUrl = pgtype.Text{String: *req.AvatarURL, Valid: true}
	}
	if req.RuntimeConfig != nil {
		if err := validateRuntimeConfig(req.RuntimeConfig); err != nil {
			writeError(w, http.StatusBadRequest, "runtime_config: "+err.Error())
			return
		}
		rc, _ := json.Marshal(req.RuntimeConfig)
		params.RuntimeConfig = rc
	}
	if req.RuntimeID != nil {
		runtime, err := h.Queries.GetAgentRuntimeForWorkspace(r.Context(), db.GetAgentRuntimeForWorkspaceParams{
			ID:          parseUUID(*req.RuntimeID),
			WorkspaceID: agent.WorkspaceID,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid runtime_id")
			return
		}
		params.RuntimeID = runtime.ID
		params.RuntimeMode = pgtype.Text{String: runtime.RuntimeMode, Valid: true}
	}
	if req.Visibility != nil {
		params.Visibility = pgtype.Text{String: *req.Visibility, Valid: true}
	}
	if req.Status != nil {
		params.Status = pgtype.Text{String: *req.Status, Valid: true}
	}
	if req.MaxConcurrentTasks != nil {
		params.MaxConcurrentTasks = pgtype.Int4{Int32: *req.MaxConcurrentTasks, Valid: true}
	}

	agent, err := h.Queries.UpdateAgent(r.Context(), params)
	if err != nil {
		slog.Warn("update agent failed", append(logger.RequestAttrs(r), "error", err, "agent_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to update agent: "+err.Error())
		return
	}

	// Handle budget changes separately so absent / null / number are all
	// distinguishable. Same three-state pattern as reports_to below.
	if len(req.BudgetMonthlyCents) > 0 {
		var cents *int64
		if err := json.Unmarshal(req.BudgetMonthlyCents, &cents); err != nil {
			writeError(w, http.StatusBadRequest, "budget_monthly_cents: must be a non-negative integer or null")
			return
		}
		var bc pgtype.Int8
		if cents != nil {
			if *cents < 0 {
				writeError(w, http.StatusBadRequest, "budget_monthly_cents must be non-negative")
				return
			}
			bc = pgtype.Int8{Int64: *cents, Valid: true}
		}
		if updated, err := h.Queries.SetAgentBudget(r.Context(), db.SetAgentBudgetParams{
			ID:          agent.ID,
			BudgetCents: bc,
		}); err != nil {
			slog.Warn("set agent budget failed", append(logger.RequestAttrs(r), "error", err, "agent_id", id)...)
			writeError(w, http.StatusInternalServerError, "failed to set budget: "+err.Error())
			return
		} else {
			agent = updated
		}
	}

	// Handle supervisor changes separately so we can distinguish "absent
	// from payload" from "explicit null to clear". len == 0 means the field
	// was omitted — leave the current link alone.
	if len(req.ReportsTo) > 0 {
		var supervisor *string
		if err := json.Unmarshal(req.ReportsTo, &supervisor); err != nil {
			writeError(w, http.StatusBadRequest, "reports_to: must be a UUID string or null")
			return
		}
		var newSupervisorID pgtype.UUID
		if supervisor != nil && *supervisor != "" {
			if _, err := h.validateSupervisor(r, *supervisor, agent.WorkspaceID); err != nil {
				writeError(w, http.StatusBadRequest, "reports_to: "+err.Error())
				return
			}
			newSupervisorID = parseUUID(*supervisor)
			if err := h.checkNoSupervisorCycle(r, agent.ID, newSupervisorID); err != nil {
				writeError(w, http.StatusBadRequest, "reports_to: "+err.Error())
				return
			}
		}
		agent, err = h.Queries.SetAgentSupervisor(r.Context(), db.SetAgentSupervisorParams{
			ID:           agent.ID,
			SupervisorID: newSupervisorID,
		})
		if err != nil {
			slog.Warn("set supervisor failed", append(logger.RequestAttrs(r), "error", err, "agent_id", id)...)
			writeError(w, http.StatusInternalServerError, "failed to set supervisor: "+err.Error())
			return
		}
	}

	resp := agentToResponse(agent)
	slog.Info("agent updated", append(logger.RequestAttrs(r), "agent_id", id, "workspace_id", uuidToString(agent.WorkspaceID))...)
	userID := requestUserID(r)
	actorType, actorID := h.resolveActor(r, userID, uuidToString(agent.WorkspaceID))
	h.publish(protocol.EventAgentStatus, uuidToString(agent.WorkspaceID), actorType, actorID, map[string]any{"agent": resp})
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ArchiveAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, ok := h.loadAgentForUser(w, r, id)
	if !ok {
		return
	}
	if !h.canManageAgent(w, r, agent) {
		return
	}
	if agent.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "agent is already archived")
		return
	}

	userID := requestUserID(r)
	archived, err := h.Queries.ArchiveAgent(r.Context(), db.ArchiveAgentParams{
		ID:         parseUUID(id),
		ArchivedBy: parseUUID(userID),
	})
	if err != nil {
		slog.Warn("archive agent failed", append(logger.RequestAttrs(r), "error", err, "agent_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to archive agent")
		return
	}

	// Cancel all pending/active tasks for this agent.
	if err := h.Queries.CancelAgentTasksByAgent(r.Context(), parseUUID(id)); err != nil {
		slog.Warn("cancel agent tasks on archive failed", append(logger.RequestAttrs(r), "error", err, "agent_id", id)...)
	}

	wsID := uuidToString(archived.WorkspaceID)
	slog.Info("agent archived", append(logger.RequestAttrs(r), "agent_id", id, "workspace_id", wsID)...)
	resp := agentToResponse(archived)
	actorType, actorID := h.resolveActor(r, userID, wsID)
	h.publish(protocol.EventAgentArchived, wsID, actorType, actorID, map[string]any{"agent": resp})
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RestoreAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, ok := h.loadAgentForUser(w, r, id)
	if !ok {
		return
	}
	if !h.canManageAgent(w, r, agent) {
		return
	}
	if !agent.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "agent is not archived")
		return
	}

	restored, err := h.Queries.RestoreAgent(r.Context(), parseUUID(id))
	if err != nil {
		slog.Warn("restore agent failed", append(logger.RequestAttrs(r), "error", err, "agent_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to restore agent")
		return
	}

	wsID := uuidToString(restored.WorkspaceID)
	slog.Info("agent restored", append(logger.RequestAttrs(r), "agent_id", id, "workspace_id", wsID)...)
	resp := agentToResponse(restored)
	userID := requestUserID(r)
	actorType, actorID := h.resolveActor(r, userID, wsID)
	h.publish(protocol.EventAgentRestored, wsID, actorType, actorID, map[string]any{"agent": resp})
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListAgentTasks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.loadAgentForUser(w, r, id); !ok {
		return
	}

	tasks, err := h.Queries.ListAgentTasks(r.Context(), parseUUID(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent tasks")
		return
	}

	resp := make([]AgentTaskResponse, len(tasks))
	for i, t := range tasks {
		resp[i] = taskToResponse(t)
	}

	writeJSON(w, http.StatusOK, resp)
}
