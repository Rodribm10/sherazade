package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const supportPilotAppKey = "inaudit"

type createSupportSessionRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Description    string `json:"description"`
	DeferAnalysis  bool   `json:"defer_analysis,omitempty"`
}

type supportCaseResponse struct {
	ID         string `json:"id"`
	PublicCode string `json:"public_code"`
	SessionID  string `json:"session_id"`
	AppKey     string `json:"app_key"`
	State      string `json:"state"`
}

type supportMessageRequest struct {
	Content       string   `json:"content"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
}

type supportSettings struct {
	Support struct {
		ConciergeAgentID   string `json:"concierge_agent_id"`
		KnowledgeContext   string `json:"knowledge_context"`
		Model              string `json:"model"`
		ProjectID          string `json:"project_id"`
		TechnicalProjectID string `json:"technical_project_id"`
		ApproverUserID     string `json:"approver_user_id"`
	} `json:"support"`
}

func supportCaseToResponse(c db.SupportCase) supportCaseResponse {
	return supportCaseResponse{
		ID:         uuidToString(c.ID),
		PublicCode: c.PublicCode,
		SessionID:  uuidToString(c.ChatSessionID),
		AppKey:     c.AppKey,
		State:      c.State,
	}
}

func (h *Handler) configuredSupportConcierge(r *http.Request, workspaceID pgtype.UUID) (db.Agent, bool) {
	return configuredSupportConcierge(r, h.Queries, workspaceID)
}

func configuredSupportConcierge(r *http.Request, queries *db.Queries, workspaceID pgtype.UUID) (db.Agent, bool) {
	settings, err := loadSupportSettings(r.Context(), queries, workspaceID)
	if err != nil {
		return db.Agent{}, false
	}
	conciergeID, err := parseUUIDValue(strings.TrimSpace(settings.Support.ConciergeAgentID))
	if err != nil {
		return db.Agent{}, false
	}
	agent, err := queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          conciergeID,
		WorkspaceID: workspaceID,
	})
	if err != nil || agent.ArchivedAt.Valid {
		return db.Agent{}, false
	}
	return agent, true
}

func loadSupportSettings(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (supportSettings, error) {
	workspace, err := queries.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return supportSettings{}, err
	}
	var settings supportSettings
	if err := json.Unmarshal(workspace.Settings, &settings); err != nil {
		return supportSettings{}, err
	}
	return settings, nil
}

func parseUUIDValue(value string) (pgtype.UUID, error) {
	if value == "" {
		return pgtype.UUID{}, errors.New("missing UUID")
	}
	parsed, ok := parseUUIDString(value)
	if !ok {
		return pgtype.UUID{}, errors.New("invalid UUID")
	}
	return parsed, nil
}

// parseUUIDString keeps the support configuration parser non-panicking because
// workspace settings are operator-managed JSON, not a trusted UUID round-trip.
func parseUUIDString(value string) (pgtype.UUID, bool) {
	var parsed pgtype.UUID
	if err := parsed.Scan(value); err != nil || !parsed.Valid {
		return pgtype.UUID{}, false
	}
	return parsed, true
}

func isSupportIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "support_case_idempotency_unique"
}

func (h *Handler) CreateSupportSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	if member.Role != "reporter" {
		writeError(w, http.StatusForbidden, "support is available only to reporters")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req createSupportSessionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 {
		writeError(w, http.StatusBadRequest, "idempotency_key must be between 8 and 128 characters")
		return
	}
	req.Description = strings.TrimSpace(req.Description)
	if req.Description == "" || len(req.Description) > 8*1024 {
		writeError(w, http.StatusBadRequest, "description must be between 1 and 8192 characters")
		return
	}

	concierge, configured := h.configuredSupportConcierge(r, workspaceUUID)
	if !configured {
		writeError(w, http.StatusServiceUnavailable, "support concierge is not configured")
		return
	}

	reporterUUID := parseUUID(userID)
	if existing, err := h.Queries.GetSupportCaseByIdempotency(r.Context(), db.GetSupportCaseByIdempotencyParams{
		WorkspaceID: workspaceUUID, ReporterUserID: reporterUUID, IdempotencyKey: req.IdempotencyKey,
	}); err == nil {
		existing = h.ensureSupportIssue(r.Context(), existing, false)
		if !req.DeferAnalysis {
			existing = h.maybeAnalyzeSupportCase(existing)
		}
		writeJSON(w, http.StatusOK, supportCaseToResponse(existing))
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load support case")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start support session")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)
	if _, err := qtx.LockWorkspaceForChatSessionCreate(r.Context(), workspaceUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock workspace")
		return
	}
	// Re-check inside the transaction to avoid duplicate work when a prior
	// request committed between the preflight read and transaction start. The
	// workspace lock protects create-vs-delete, but is intentionally shared by
	// creators; the unique idempotency index below arbitrates true races.
	if existing, err := qtx.GetSupportCaseByIdempotency(r.Context(), db.GetSupportCaseByIdempotencyParams{
		WorkspaceID: workspaceUUID, ReporterUserID: reporterUUID, IdempotencyKey: req.IdempotencyKey,
	}); err == nil {
		if rollbackErr := tx.Rollback(r.Context()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			writeError(w, http.StatusInternalServerError, "failed to resolve support case retry")
			return
		}
		existing = h.ensureSupportIssue(r.Context(), existing, false)
		if !req.DeferAnalysis {
			existing = h.maybeAnalyzeSupportCase(existing)
		}
		writeJSON(w, http.StatusOK, supportCaseToResponse(existing))
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load support case")
		return
	}
	// Re-read configuration in the locked transaction so an archived, deleted,
	// or reconfigured Concierge cannot be used after the preflight check.
	concierge, configured = configuredSupportConcierge(r, qtx, workspaceUUID)
	if !configured {
		writeError(w, http.StatusServiceUnavailable, "support concierge is not configured")
		return
	}
	session, err := qtx.CreateChatSession(r.Context(), db.CreateChatSessionParams{
		WorkspaceID: workspaceUUID, AgentID: concierge.ID, CreatorID: reporterUUID,
		Title: "Support: InAudit", ProjectID: pgtype.UUID{},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create support session")
		return
	}
	initialMessage, err := qtx.CreateChatMessage(r.Context(), db.CreateChatMessageParams{
		ChatSessionID: session.ID,
		Role:          "user",
		Content:       req.Description,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create initial support message")
		return
	}
	sequence, err := qtx.NextSupportCasePublicSequence(r.Context(), workspaceUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to allocate support case code")
		return
	}
	caseRow, err := qtx.CreateSupportCase(r.Context(), db.CreateSupportCaseParams{
		WorkspaceID: workspaceUUID, CaseNumber: strconv.FormatInt(sequence, 10), ReporterUserID: reporterUUID,
		ChatSessionID: session.ID, PendingMessageID: initialMessage.ID, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		if isSupportIdempotencyConflict(err) {
			// The competing transaction has committed before PostgreSQL reports
			// the unique violation. Roll back this transaction (including its
			// chat, initial message, and sequence increment), then return the
			// winner as an ordinary idempotent retry.
			if rollbackErr := tx.Rollback(r.Context()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				writeError(w, http.StatusInternalServerError, "failed to resolve support case retry")
				return
			}
			existing, loadErr := h.Queries.GetSupportCaseByIdempotency(r.Context(), db.GetSupportCaseByIdempotencyParams{
				WorkspaceID: workspaceUUID, ReporterUserID: reporterUUID, IdempotencyKey: req.IdempotencyKey,
			})
			if loadErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to load support case retry")
				return
			}
			existing = h.ensureSupportIssue(r.Context(), existing, false)
			if !req.DeferAnalysis {
				existing = h.maybeAnalyzeSupportCase(existing)
			}
			writeJSON(w, http.StatusOK, supportCaseToResponse(existing))
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create support case")
		return
	}
	if _, err := qtx.CreateSupportCaseTransition(r.Context(), db.CreateSupportCaseTransitionParams{
		SupportCaseID: caseRow.ID, PreviousState: pgtype.Text{}, NewState: "novo", ActorType: "member", ActorID: reporterUUID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to audit support case")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit support session")
		return
	}
	caseRow = h.ensureSupportIssue(r.Context(), caseRow, false)
	if !req.DeferAnalysis {
		caseRow = h.maybeAnalyzeSupportCase(caseRow)
	}
	writeJSON(w, http.StatusCreated, supportCaseToResponse(caseRow))
}

func (h *Handler) ListSupportSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	rows, err := h.Queries.ListSupportCasesForReporter(r.Context(), db.ListSupportCasesForReporterParams{WorkspaceID: parseUUID(workspaceID), ReporterUserID: parseUUID(userID)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list support sessions")
		return
	}
	resp := make([]supportCaseResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, supportCaseToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetSupportCase(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "support case id")
	if !ok {
		return
	}
	caseRow, err := h.Queries.GetSupportCaseForReporter(r.Context(), db.GetSupportCaseForReporterParams{ID: id, WorkspaceID: parseUUID(ctxWorkspaceID(r.Context())), ReporterUserID: parseUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "support case not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support case")
		return
	}
	caseRow = h.maybeAnalyzeSupportCase(caseRow)
	writeJSON(w, http.StatusOK, supportCaseToResponse(caseRow))
}

func (h *Handler) GetSupportSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	sessionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "support session id")
	if !ok {
		return
	}
	caseRow, err := h.Queries.GetSupportCaseBySessionForReporter(r.Context(), db.GetSupportCaseBySessionForReporterParams{
		ChatSessionID: sessionID, WorkspaceID: parseUUID(ctxWorkspaceID(r.Context())), ReporterUserID: parseUUID(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "support session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support session")
		return
	}
	caseRow = h.maybeAnalyzeSupportCase(caseRow)
	writeJSON(w, http.StatusOK, supportCaseToResponse(caseRow))
}

func (h *Handler) ConfirmSupportResolution(w http.ResponseWriter, r *http.Request) {
	h.updateSupportResolutionByReporter(w, r, true)
}

func (h *Handler) ReopenSupportResolution(w http.ResponseWriter, r *http.Request) {
	h.updateSupportResolutionByReporter(w, r, false)
}

func (h *Handler) updateSupportResolutionByReporter(w http.ResponseWriter, r *http.Request, resolved bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	caseID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "support case id")
	if !ok {
		return
	}
	workspaceID := parseUUID(ctxWorkspaceID(r.Context()))
	reporterID := parseUUID(userID)
	owned, err := h.Queries.GetSupportCaseForReporter(r.Context(), db.GetSupportCaseForReporterParams{
		ID: caseID, WorkspaceID: workspaceID, ReporterUserID: reporterID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "support case not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support case")
		return
	}

	allowed := false
	if resolved {
		allowed = owned.State == "aguardando_confirmacao" || owned.State == "resposta_proposta"
	} else {
		allowed = owned.State == "aguardando_confirmacao" || owned.State == "resposta_proposta" || owned.State == "concluido"
	}
	if !allowed {
		writeError(w, http.StatusConflict, "support case is not awaiting resolution feedback")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update support case")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockSupportCase(r.Context(), db.LockSupportCaseParams{ID: caseID, WorkspaceID: workspaceID})
	if err != nil || locked.ReporterUserID != reporterID {
		writeError(w, http.StatusNotFound, "support case not found")
		return
	}
	if resolved {
		allowed = locked.State == "aguardando_confirmacao" || locked.State == "resposta_proposta"
	} else {
		allowed = locked.State == "aguardando_confirmacao" || locked.State == "resposta_proposta" || locked.State == "concluido"
	}
	if !allowed {
		writeError(w, http.StatusConflict, "support case changed before feedback was recorded")
		return
	}

	nextState := "aguardando_relator"
	var updated db.SupportCase
	if resolved {
		nextState = "concluido"
		updated, err = qtx.ConfirmSupportCaseResolution(r.Context(), db.ConfirmSupportCaseResolutionParams{ID: caseID, WorkspaceID: workspaceID})
	} else {
		updated, err = qtx.ReopenSupportCaseForReporter(r.Context(), db.ReopenSupportCaseForReporterParams{ID: caseID, WorkspaceID: workspaceID})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update support case")
		return
	}
	if _, err := qtx.CreateSupportCaseTransition(r.Context(), db.CreateSupportCaseTransitionParams{
		SupportCaseID: caseID,
		PreviousState: pgtype.Text{String: locked.State, Valid: true},
		NewState:      nextState,
		ActorType:     "member",
		ActorID:       reporterID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to audit support feedback")
		return
	}
	if !resolved {
		if _, err := qtx.CreateChatMessage(r.Context(), db.CreateChatMessageParams{
			ChatSessionID: locked.ChatSessionID,
			Role:          "assistant",
			Content:       "Entendi. Conte o que ainda não resolveu ou envie outro print; o caso foi reaberto.",
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reopen support conversation")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit support feedback")
		return
	}
	h.syncSupportIssueStatuses(r.Context(), updated)
	writeJSON(w, http.StatusOK, supportCaseToResponse(updated))
}

func (h *Handler) supportSessionForReporter(w http.ResponseWriter, r *http.Request) (db.SupportCase, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return db.SupportCase{}, false
	}
	sessionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "support session id")
	if !ok {
		return db.SupportCase{}, false
	}
	caseRow, err := h.Queries.GetSupportCaseBySessionForReporter(r.Context(), db.GetSupportCaseBySessionForReporterParams{ChatSessionID: sessionID, WorkspaceID: parseUUID(ctxWorkspaceID(r.Context())), ReporterUserID: parseUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "support session not found")
		return db.SupportCase{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support session")
		return db.SupportCase{}, false
	}
	return caseRow, true
}

func (h *Handler) ListSupportMessages(w http.ResponseWriter, r *http.Request) {
	caseRow, ok := h.supportSessionForReporter(w, r)
	if !ok {
		return
	}
	messages, err := h.Queries.ListChatMessages(r.Context(), caseRow.ChatSessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list support messages")
		return
	}
	messageIDs := make([]pgtype.UUID, 0, len(messages))
	for _, message := range messages {
		messageIDs = append(messageIDs, message.ID)
	}
	attachments := h.groupChatMessageAttachments(r.Context(), uuidToString(caseRow.WorkspaceID), messageIDs)
	resp := make([]ChatMessageResponse, len(messages))
	for i, message := range messages {
		resp[i] = chatMessageToResponse(message, attachments[uuidToString(message.ID)])
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SendSupportMessage(w http.ResponseWriter, r *http.Request) {
	caseRow, ok := h.supportSessionForReporter(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req supportMessageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if len(req.Content) > 8*1024 {
		writeError(w, http.StatusBadRequest, "content must not exceed 8192 characters")
		return
	}
	attachmentIDs, ok := parseUUIDSliceOrBadRequest(w, req.AttachmentIDs, "attachment_ids")
	if !ok {
		return
	}
	if len(attachmentIDs) > 5 {
		writeError(w, http.StatusBadRequest, "attachment_ids must not contain more than 5 files")
		return
	}
	if req.Content == "" && len(attachmentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "content or attachment_ids is required")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start support message")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockSupportCase(r.Context(), db.LockSupportCaseParams{ID: caseRow.ID, WorkspaceID: caseRow.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "support session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock support session")
		return
	}
	messageContent := req.Content
	if messageContent == "" {
		messageContent = "Enviei um anexo para complementar o atendimento."
	}
	message, err := qtx.CreateChatMessage(r.Context(), db.CreateChatMessageParams{ChatSessionID: locked.ChatSessionID, Role: "user", Content: messageContent})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create support message")
		return
	}
	if len(attachmentIDs) > 0 {
		linked, err := qtx.LinkAttachmentsToChatMessage(r.Context(), db.LinkAttachmentsToChatMessageParams{
			ChatMessageID: message.ID,
			ChatSessionID: locked.ChatSessionID,
			WorkspaceID:   locked.WorkspaceID,
			UploaderType:  "member",
			UploaderID:    locked.ReporterUserID,
			AttachmentIds: attachmentIDs,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to link support attachments")
			return
		}
		if len(linked) != len(attachmentIDs) {
			writeError(w, http.StatusBadRequest, "one or more attachments do not belong to this support session")
			return
		}
	}
	updated, err := qtx.MarkSupportCasePending(r.Context(), db.MarkSupportCasePendingParams{
		PendingMessageID: message.ID,
		ID:               locked.ID,
		WorkspaceID:      locked.WorkspaceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue support analysis")
		return
	}
	if _, err := qtx.CreateSupportCaseTransition(r.Context(), db.CreateSupportCaseTransitionParams{
		SupportCaseID: locked.ID,
		PreviousState: pgtype.Text{String: locked.State, Valid: true},
		NewState:      "coletando_contexto",
		ActorType:     "member",
		ActorID:       locked.ReporterUserID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to audit support message")
		return
	}
	if err := qtx.TouchChatSession(r.Context(), locked.ChatSessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update support session")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit support message")
		return
	}
	h.maybeAnalyzeSupportCase(updated)
	var responseAttachments []AttachmentResponse
	if len(attachmentIDs) > 0 {
		grouped := h.groupChatMessageAttachments(r.Context(), uuidToString(locked.WorkspaceID), []pgtype.UUID{message.ID})
		responseAttachments = grouped[uuidToString(message.ID)]
	}
	writeJSON(w, http.StatusCreated, chatMessageToResponse(message, responseAttachments))
}
