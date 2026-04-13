package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// Chat Sessions
// ---------------------------------------------------------------------------

type CreateChatSessionRequest struct {
	AgentID string `json:"agent_id"`
	Title   string `json:"title"`
	// CreateIssue switches to the "chat-first" flow: the backend
	// creates an issue on the spot (title + Prompt as description,
	// assignee=agent), links this chat session to it, and routes
	// future messages through issue comments so the conversation
	// shows up in the Kanban, Inbox, and issue detail view.
	// Falsy preserves the legacy free-floating chat behavior.
	CreateIssue bool `json:"create_issue"`
	// Prompt is the initial task description when CreateIssue=true.
	// Required in that mode; ignored otherwise. The first ~60 chars
	// also become the issue title when Title is empty.
	Prompt string `json:"prompt"`
}

func (h *Handler) CreateChatSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())

	var req CreateChatSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	// Verify agent exists in workspace.
	agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          parseUUID(req.AgentID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if agent.ArchivedAt.Valid {
		writeError(w, http.StatusBadRequest, "agent is archived")
		return
	}

	// Legacy flow: free-floating chat session without an issue.
	if !req.CreateIssue {
		session, err := h.Queries.CreateChatSession(r.Context(), db.CreateChatSessionParams{
			WorkspaceID: parseUUID(workspaceID),
			AgentID:     parseUUID(req.AgentID),
			CreatorID:   parseUUID(userID),
			Title:       req.Title,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create chat session")
			return
		}
		writeJSON(w, http.StatusCreated, chatSessionToResponse(session))
		return
	}

	// Chat-first flow: create an issue + link the session to it.
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required when create_issue is true")
		return
	}
	title := req.Title
	if title == "" {
		title = req.Prompt
		if len(title) > 80 {
			title = title[:80]
		}
	}

	// Issue counter is transactional to avoid duplicate numbers.
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create issue")
		return
	}
	defer tx.Rollback(r.Context())

	qtx := h.Queries.WithTx(tx)
	issueNumber, err := qtx.IncrementIssueCounter(r.Context(), parseUUID(workspaceID))
	if err != nil {
		slog.Warn("chat-first: increment counter failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create issue")
		return
	}

	issue, err := qtx.CreateIssue(r.Context(), db.CreateIssueParams{
		WorkspaceID:  parseUUID(workspaceID),
		Title:        title,
		Description:  pgtype.Text{String: req.Prompt, Valid: true},
		Status:       "todo",
		Priority:     "none",
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   agent.ID,
		CreatorType:  "user",
		CreatorID:    parseUUID(userID),
		Number:       issueNumber,
	})
	if err != nil {
		slog.Warn("chat-first: create issue failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create issue")
		return
	}

	session, err := qtx.CreateChatSession(r.Context(), db.CreateChatSessionParams{
		WorkspaceID: parseUUID(workspaceID),
		AgentID:     parseUUID(req.AgentID),
		CreatorID:   parseUUID(userID),
		Title:       title,
		IssueID:     pgtype.UUID{Bytes: issue.ID.Bytes, Valid: issue.ID.Valid},
	})
	if err != nil {
		slog.Warn("chat-first: create session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create chat session")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit chat-first creation")
		return
	}

	// Outside the transaction: enqueue the initial agent task on the
	// new issue. Uses the same path a manual assign would take, so the
	// team-awareness prompt, workdir-direct mode, etc. all apply.
	resp := chatSessionToResponse(session)
	if task, err := h.TaskService.EnqueueTaskForIssue(r.Context(), issue); err != nil {
		slog.Warn("chat-first: enqueue task failed (issue created, task not queued)",
			"error", err,
			"issue_id", uuidToString(issue.ID),
		)
	} else {
		taskID := uuidToString(task.ID)
		resp.InitialTaskID = &taskID
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) ListChatSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())

	status := r.URL.Query().Get("status")

	var sessions []db.ChatSession
	var err error
	if status == "all" {
		sessions, err = h.Queries.ListAllChatSessionsByCreator(r.Context(), db.ListAllChatSessionsByCreatorParams{
			WorkspaceID: parseUUID(workspaceID),
			CreatorID:   parseUUID(userID),
		})
	} else {
		sessions, err = h.Queries.ListChatSessionsByCreator(r.Context(), db.ListChatSessionsByCreatorParams{
			WorkspaceID: parseUUID(workspaceID),
			CreatorID:   parseUUID(userID),
		})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list chat sessions")
		return
	}

	resp := make([]ChatSessionResponse, len(sessions))
	for i, s := range sessions {
		resp[i] = chatSessionToResponse(s)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetChatSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	session, err := h.Queries.GetChatSessionInWorkspace(r.Context(), db.GetChatSessionInWorkspaceParams{
		ID:          parseUUID(sessionID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "chat session not found")
		return
	}
	if uuidToString(session.CreatorID) != userID {
		writeError(w, http.StatusForbidden, "not your chat session")
		return
	}

	writeJSON(w, http.StatusOK, chatSessionToResponse(session))
}

func (h *Handler) ArchiveChatSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	session, err := h.Queries.GetChatSessionInWorkspace(r.Context(), db.GetChatSessionInWorkspaceParams{
		ID:          parseUUID(sessionID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "chat session not found")
		return
	}
	if uuidToString(session.CreatorID) != userID {
		writeError(w, http.StatusForbidden, "not your chat session")
		return
	}

	if err := h.Queries.ArchiveChatSession(r.Context(), parseUUID(sessionID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to archive chat session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Chat Messages
// ---------------------------------------------------------------------------

type SendChatMessageRequest struct {
	Content string `json:"content"`
}

type SendChatMessageResponse struct {
	MessageID string `json:"message_id"`
	TaskID    string `json:"task_id"`
}

func (h *Handler) SendChatMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	var req SendChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Load chat session.
	session, err := h.Queries.GetChatSessionInWorkspace(r.Context(), db.GetChatSessionInWorkspaceParams{
		ID:          parseUUID(sessionID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "chat session not found")
		return
	}
	if session.Status != "active" {
		writeError(w, http.StatusBadRequest, "chat session is archived")
		return
	}
	if uuidToString(session.CreatorID) != userID {
		writeError(w, http.StatusForbidden, "not your chat session")
		return
	}

	// Chat-first flow: if this session is linked to an issue, route
	// every message through the issue's comment thread. The comment
	// itself becomes the trigger for a new agent task, so the
	// conversation shows up in the issue detail view, the Kanban,
	// and the Inbox just like any human-driven conversation.
	if session.IssueID.Valid {
		issue, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{
			ID:          session.IssueID,
			WorkspaceID: parseUUID(workspaceID),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "linked issue not found")
			return
		}
		comment, err := h.Queries.CreateComment(r.Context(), db.CreateCommentParams{
			IssueID:     issue.ID,
			WorkspaceID: issue.WorkspaceID,
			AuthorType:  "user",
			AuthorID:    parseUUID(userID),
			Content:     req.Content,
			Type:        "comment",
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to post comment")
			return
		}
		task, err := h.TaskService.EnqueueTaskForIssue(r.Context(), issue, comment.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to enqueue task: "+err.Error())
			return
		}
		if err := h.Queries.TouchChatSession(r.Context(), session.ID); err != nil {
			slog.Warn("failed to touch chat session", "session_id", sessionID, "error", err)
		}
		writeJSON(w, http.StatusCreated, SendChatMessageResponse{
			MessageID: uuidToString(comment.ID),
			TaskID:    uuidToString(task.ID),
		})
		return
	}

	// Legacy free-floating chat flow.
	// Create the user message first so the daemon can always find it.
	msg, err := h.Queries.CreateChatMessage(r.Context(), db.CreateChatMessageParams{
		ChatSessionID: session.ID,
		Role:          "user",
		Content:       req.Content,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create chat message")
		return
	}

	// Enqueue a chat task after the message exists.
	task, err := h.TaskService.EnqueueChatTask(r.Context(), session)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue chat task: "+err.Error())
		return
	}

	// Touch session updated_at.
	if err := h.Queries.TouchChatSession(r.Context(), session.ID); err != nil {
		slog.Warn("failed to touch chat session", "session_id", sessionID, "error", err)
	}

	// Broadcast the user message.
	h.publish(protocol.EventChatMessage, workspaceID, "member", userID, protocol.ChatMessagePayload{
		ChatSessionID: sessionID,
		MessageID:     uuidToString(msg.ID),
		Role:          "user",
		Content:       req.Content,
		TaskID:        uuidToString(task.ID),
		CreatedAt:     timestampToString(msg.CreatedAt),
	})

	writeJSON(w, http.StatusCreated, SendChatMessageResponse{
		MessageID: uuidToString(msg.ID),
		TaskID:    uuidToString(task.ID),
	})
}

func (h *Handler) ListChatMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	session, err := h.Queries.GetChatSessionInWorkspace(r.Context(), db.GetChatSessionInWorkspaceParams{
		ID:          parseUUID(sessionID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "chat session not found")
		return
	}
	if uuidToString(session.CreatorID) != userID {
		writeError(w, http.StatusForbidden, "not your chat session")
		return
	}

	// Chat-first sessions store their conversation as issue comments.
	// Return the issue description as the seed user message, followed
	// by every comment (author_type maps to role: user or assistant),
	// so the existing ChatMessageList in the frontend renders it
	// unchanged.
	if session.IssueID.Valid {
		issue, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{
			ID:          session.IssueID,
			WorkspaceID: parseUUID(workspaceID),
		})
		if err != nil {
			writeError(w, http.StatusNotFound, "linked issue not found")
			return
		}
		comments, err := h.Queries.ListComments(r.Context(), db.ListCommentsParams{
			IssueID:     session.IssueID,
			WorkspaceID: parseUUID(workspaceID),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list issue comments")
			return
		}
		resp := make([]ChatMessageResponse, 0, len(comments)+1)
		// Seed: the issue description is the initial user prompt.
		if issue.Description.Valid && issue.Description.String != "" {
			resp = append(resp, ChatMessageResponse{
				ID:            uuidToString(issue.ID),
				ChatSessionID: sessionID,
				Role:          "user",
				Content:       issue.Description.String,
				CreatedAt:     timestampToString(issue.CreatedAt),
			})
		}
		for _, c := range comments {
			role := "user"
			if c.AuthorType == "agent" {
				role = "assistant"
			}
			resp = append(resp, ChatMessageResponse{
				ID:            uuidToString(c.ID),
				ChatSessionID: sessionID,
				Role:          role,
				Content:       c.Content,
				CreatedAt:     timestampToString(c.CreatedAt),
			})
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	messages, err := h.Queries.ListChatMessages(r.Context(), parseUUID(sessionID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list chat messages")
		return
	}

	resp := make([]ChatMessageResponse, len(messages))
	for i, m := range messages {
		resp[i] = chatMessageToResponse(m)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Task cancellation (user-facing, with ownership check)
// ---------------------------------------------------------------------------

// CancelTaskByUser cancels a task after verifying the requesting user owns
// the associated chat session or issue within the current workspace.
func (h *Handler) CancelTaskByUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	taskID := chi.URLParam(r, "taskId")

	task, err := h.Queries.GetAgentTask(r.Context(), parseUUID(taskID))
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Verify ownership: for chat tasks, check workspace + creator;
	// for issue tasks, verify the issue belongs to the current workspace.
	if task.ChatSessionID.Valid {
		cs, err := h.Queries.GetChatSessionInWorkspace(r.Context(), db.GetChatSessionInWorkspaceParams{
			ID:          task.ChatSessionID,
			WorkspaceID: parseUUID(workspaceID),
		})
		if err != nil {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if uuidToString(cs.CreatorID) != userID {
			writeError(w, http.StatusForbidden, "not your task")
			return
		}
	} else if task.IssueID.Valid {
		issue, err := h.Queries.GetIssue(r.Context(), task.IssueID)
		if err != nil || uuidToString(issue.WorkspaceID) != workspaceID {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
	} else {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	cancelled, err := h.TaskService.CancelTask(r.Context(), parseUUID(taskID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, taskToResponse(*cancelled))
}

// ---------------------------------------------------------------------------
// Response types & helpers
// ---------------------------------------------------------------------------

type ChatSessionResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	AgentID     string  `json:"agent_id"`
	CreatorID   string  `json:"creator_id"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	// IssueID is set when this chat session is backed by an issue
	// (chat-first flow). Null for legacy free-floating chats.
	IssueID   *string `json:"issue_id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	// InitialTaskID is populated only in the chat-first create response,
	// so the frontend can show a running indicator until the first reply
	// lands. Not persisted; omitted on subsequent reads.
	InitialTaskID *string `json:"initial_task_id,omitempty"`
}

type ChatMessageResponse struct {
	ID            string  `json:"id"`
	ChatSessionID string  `json:"chat_session_id"`
	Role          string  `json:"role"`
	Content       string  `json:"content"`
	TaskID        *string `json:"task_id"`
	CreatedAt     string  `json:"created_at"`
}

func chatSessionToResponse(s db.ChatSession) ChatSessionResponse {
	return ChatSessionResponse{
		ID:          uuidToString(s.ID),
		WorkspaceID: uuidToString(s.WorkspaceID),
		AgentID:     uuidToString(s.AgentID),
		CreatorID:   uuidToString(s.CreatorID),
		Title:       s.Title,
		Status:      s.Status,
		IssueID:     uuidToPtr(s.IssueID),
		CreatedAt:   timestampToString(s.CreatedAt),
		UpdatedAt:   timestampToString(s.UpdatedAt),
	}
}

func chatMessageToResponse(m db.ChatMessage) ChatMessageResponse {
	return ChatMessageResponse{
		ID:            uuidToString(m.ID),
		ChatSessionID: uuidToString(m.ChatSessionID),
		Role:          m.Role,
		Content:       m.Content,
		TaskID:        uuidToPtr(m.TaskID),
		CreatedAt:     timestampToString(m.CreatedAt),
	}
}
