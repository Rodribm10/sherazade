package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type supportAdminCaseResponse struct {
	ID                string  `json:"id"`
	PublicCode        string  `json:"public_code"`
	ReporterUserID    string  `json:"reporter_user_id"`
	SessionID         string  `json:"session_id"`
	SupportIssueID    *string `json:"support_issue_id"`
	TechnicalIssueID  *string `json:"technical_issue_id"`
	AppKey            string  `json:"app_key"`
	State             string  `json:"state"`
	RiskLevel         *string `json:"risk_level"`
	Confidence        *string `json:"confidence"`
	ResolutionType    *string `json:"resolution_type"`
	ResolutionSummary *string `json:"resolution_summary"`
	ApprovalRevision  int32   `json:"approval_revision"`
	ApprovalSummary   *string `json:"approval_summary"`
	ApprovalBy        *string `json:"approval_by"`
	ApprovalAt        *string `json:"approval_at"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type supportAdminDetailResponse struct {
	Case        supportAdminCaseResponse        `json:"case"`
	Messages    []ChatMessageResponse           `json:"messages"`
	Transitions []supportCaseTransitionResponse `json:"transitions"`
}

type supportCaseTransitionResponse struct {
	PreviousState *string `json:"previous_state"`
	NewState      string  `json:"new_state"`
	ActorType     string  `json:"actor_type"`
	ActorID       string  `json:"actor_id"`
	CreatedAt     string  `json:"created_at"`
}

func supportCaseToAdminResponse(c db.SupportCase) supportAdminCaseResponse {
	return supportAdminCaseResponse{
		ID:                uuidToString(c.ID),
		PublicCode:        c.PublicCode,
		ReporterUserID:    uuidToString(c.ReporterUserID),
		SessionID:         uuidToString(c.ChatSessionID),
		SupportIssueID:    uuidToPtr(c.SupportIssueID),
		TechnicalIssueID:  uuidToPtr(c.TechnicalIssueID),
		AppKey:            c.AppKey,
		State:             c.State,
		RiskLevel:         textToPtr(c.RiskLevel),
		Confidence:        textToPtr(c.Confidence),
		ResolutionType:    textToPtr(c.ResolutionType),
		ResolutionSummary: textToPtr(c.ResolutionSummary),
		ApprovalRevision:  c.ApprovalRevision,
		ApprovalSummary:   textToPtr(c.ApprovalSummary),
		ApprovalBy:        uuidToPtr(c.ApprovalBy),
		ApprovalAt:        timestamptzToPtr(c.ApprovalAt),
		CreatedAt:         timestampToString(c.CreatedAt),
		UpdatedAt:         timestampToString(c.UpdatedAt),
	}
}

func timestamptzToPtr(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.Format("2006-01-02T15:04:05Z07:00")
	return &formatted
}

func (h *Handler) ListSupportCasesAdmin(w http.ResponseWriter, r *http.Request) {
	workspaceID := parseUUID(ctxWorkspaceID(r.Context()))
	rows, err := h.Queries.ListSupportCasesForWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list support cases")
		return
	}
	response := make([]supportAdminCaseResponse, 0, len(rows))
	for _, row := range rows {
		// Reconcile idempotently so an earlier transient project/issue failure
		// cannot leave an escalated case invisible to the operational queue.
		row = h.ensureSupportIssue(r.Context(), row, false)
		if row.State == "em_investigacao_tecnica" || row.State == "aguardando_aprovacao" || row.TechnicalIssueID.Valid {
			row = h.ensureSupportIssue(r.Context(), row, true)
		}
		response = append(response, supportCaseToAdminResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetSupportCaseAdmin(w http.ResponseWriter, r *http.Request) {
	caseRow, ok := h.supportCaseForAdmin(w, r)
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
	messageResponse := make([]ChatMessageResponse, 0, len(messages))
	for _, message := range messages {
		messageResponse = append(messageResponse, chatMessageToResponse(message, attachments[uuidToString(message.ID)]))
	}
	transitions, err := h.Queries.ListSupportCaseTransitions(r.Context(), caseRow.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list support transitions")
		return
	}
	transitionResponse := make([]supportCaseTransitionResponse, 0, len(transitions))
	for _, transition := range transitions {
		transitionResponse = append(transitionResponse, supportCaseTransitionResponse{
			PreviousState: textToPtr(transition.PreviousState),
			NewState:      transition.NewState,
			ActorType:     transition.ActorType,
			ActorID:       uuidToString(transition.ActorID),
			CreatedAt:     timestampToString(transition.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, supportAdminDetailResponse{
		Case: supportCaseToAdminResponse(caseRow), Messages: messageResponse, Transitions: transitionResponse,
	})
}

func (h *Handler) GetSupportMetricsAdmin(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.Queries.GetSupportCaseMetrics(r.Context(), parseUUID(ctxWorkspaceID(r.Context())))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support metrics")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *Handler) supportCaseForAdmin(w http.ResponseWriter, r *http.Request) (db.SupportCase, bool) {
	caseID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "support case id")
	if !ok {
		return db.SupportCase{}, false
	}
	caseRow, err := h.Queries.GetSupportCaseInWorkspace(r.Context(), db.GetSupportCaseInWorkspaceParams{
		ID: caseID, WorkspaceID: parseUUID(ctxWorkspaceID(r.Context())),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "support case not found")
		return db.SupportCase{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load support case")
		return db.SupportCase{}, false
	}
	return caseRow, true
}

type supportApprovalRequest struct {
	Summary string `json:"summary"`
}

type supportTechnicalResultRequest struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

func decodeSupportAdminJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (h *Handler) RequestSupportApproval(w http.ResponseWriter, r *http.Request) {
	var req supportApprovalRequest
	if !decodeSupportAdminJSON(w, r, &req) {
		return
	}
	req.Summary = strings.TrimSpace(req.Summary)
	if req.Summary == "" || len(req.Summary) > 8*1024 {
		writeError(w, http.StatusBadRequest, "summary must be between 1 and 8192 characters")
		return
	}
	h.updateSupportCaseAdmin(w, r, "request", req.Summary, "")
}

func (h *Handler) ApproveSupportExecution(w http.ResponseWriter, r *http.Request) {
	h.updateSupportCaseAdmin(w, r, "approve", "", "")
}

func (h *Handler) RejectSupportExecution(w http.ResponseWriter, r *http.Request) {
	h.updateSupportCaseAdmin(w, r, "reject", "", "")
}

func (h *Handler) CompleteSupportTechnicalWork(w http.ResponseWriter, r *http.Request) {
	var req supportTechnicalResultRequest
	if !decodeSupportAdminJSON(w, r, &req) {
		return
	}
	req.Status = strings.TrimSpace(req.Status)
	req.Summary = strings.TrimSpace(req.Summary)
	if !oneOf(req.Status, "validated", "blocked", "needs_rework", "published") || req.Summary == "" || len(req.Summary) > 8*1024 {
		writeError(w, http.StatusBadRequest, "invalid technical result")
		return
	}
	h.updateSupportCaseAdmin(w, r, "result", req.Summary, req.Status)
}

func (h *Handler) updateSupportCaseAdmin(w http.ResponseWriter, r *http.Request, action, summary, resultStatus string) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	caseRow, ok := h.supportCaseForAdmin(w, r)
	if !ok {
		return
	}
	actorID := parseUUID(userID)
	if action == "approve" || action == "reject" || (action == "result" && resultStatus == "published") {
		settings, err := loadSupportSettings(r.Context(), h.Queries, caseRow.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "support approver is not configured")
			return
		}
		if configured := strings.TrimSpace(settings.Support.ApproverUserID); configured != "" {
			approverID, err := parseUUIDValue(configured)
			if err != nil || !sameSupportUUID(approverID, actorID) {
				writeError(w, http.StatusForbidden, "only the configured support approver may decide this execution or publication")
				return
			}
		} else if member, present := ctxMember(r.Context()); !present || member.Role != "owner" {
			writeError(w, http.StatusForbidden, "only a workspace owner may approve support execution or publication")
			return
		}
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update support case")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)
	locked, err := qtx.LockSupportCase(r.Context(), db.LockSupportCaseParams{ID: caseRow.ID, WorkspaceID: caseRow.WorkspaceID})
	if err != nil {
		writeError(w, http.StatusNotFound, "support case not found")
		return
	}
	previous := locked.State
	var updated db.SupportCase
	switch action {
	case "request":
		if !oneOf(previous, "em_investigacao_tecnica", "em_validacao", "em_correcao") {
			writeError(w, http.StatusConflict, "support case is not ready for an approval proposal")
			return
		}
		updated, err = qtx.RequestSupportCaseApproval(r.Context(), db.RequestSupportCaseApprovalParams{
			ApprovalSummary: pgtype.Text{String: summary, Valid: true}, ID: locked.ID, WorkspaceID: locked.WorkspaceID,
		})
	case "approve":
		if previous != "aguardando_aprovacao" {
			writeError(w, http.StatusConflict, "support case is not awaiting approval")
			return
		}
		updated, err = qtx.ApproveSupportCaseExecution(r.Context(), db.ApproveSupportCaseExecutionParams{
			ApprovalBy: actorID, ID: locked.ID, WorkspaceID: locked.WorkspaceID,
		})
	case "reject":
		if previous != "aguardando_aprovacao" {
			writeError(w, http.StatusConflict, "support case is not awaiting approval")
			return
		}
		updated, err = qtx.RejectSupportCaseExecution(r.Context(), db.RejectSupportCaseExecutionParams{
			ApprovalBy: actorID, ID: locked.ID, WorkspaceID: locked.WorkspaceID,
		})
	case "result":
		nextState := map[string]string{"validated": "pronto_para_publicar", "blocked": "bloqueado", "needs_rework": "em_correcao", "published": "publicado"}[resultStatus]
		technicalResultAllowed := oneOf(previous, "em_correcao", "em_validacao") ||
			(resultStatus == "blocked" && previous == "em_investigacao_tecnica")
		if resultStatus != "published" && !technicalResultAllowed {
			writeError(w, http.StatusConflict, "support case is not in technical execution")
			return
		}
		if resultStatus == "published" && previous != "pronto_para_publicar" {
			writeError(w, http.StatusConflict, "support case is not ready to publish")
			return
		}
		updated, err = qtx.CompleteSupportTechnicalWork(r.Context(), db.CompleteSupportTechnicalWorkParams{
			State: nextState, ResolutionSummary: pgtype.Text{String: summary, Valid: true}, ID: locked.ID, WorkspaceID: locked.WorkspaceID,
		})
	default:
		writeError(w, http.StatusBadRequest, "unsupported support action")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update support case")
		return
	}
	if _, err := qtx.CreateSupportCaseTransition(r.Context(), db.CreateSupportCaseTransitionParams{
		SupportCaseID: locked.ID,
		PreviousState: pgtype.Text{String: previous, Valid: true},
		NewState:      updated.State,
		ActorType:     "member",
		ActorID:       actorID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to audit support action")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit support action")
		return
	}
	h.syncSupportIssueStatuses(r.Context(), updated)
	writeJSON(w, http.StatusOK, supportCaseToAdminResponse(updated))
}
