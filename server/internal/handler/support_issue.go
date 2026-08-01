package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func supportIssueOrigin(technical bool) string {
	if technical {
		return "support_technical"
	}
	return "support_case"
}

func (h *Handler) ensureSupportIssue(ctx context.Context, caseRow db.SupportCase, technical bool) db.SupportCase {
	if h.IssueService == nil || (technical && caseRow.TechnicalIssueID.Valid) || (!technical && caseRow.SupportIssueID.Valid) {
		return caseRow
	}
	settings, err := loadSupportSettings(ctx, h.Queries, caseRow.WorkspaceID)
	if err != nil {
		slog.Warn("support issue: load settings failed", "case_id", uuidToString(caseRow.ID), "error", err)
		return caseRow
	}
	projectValue := strings.TrimSpace(settings.Support.ProjectID)
	if technical && strings.TrimSpace(settings.Support.TechnicalProjectID) != "" {
		projectValue = strings.TrimSpace(settings.Support.TechnicalProjectID)
	}
	projectID, err := parseUUIDValue(projectValue)
	if err != nil {
		slog.Warn("support issue: project is not configured", "case_id", uuidToString(caseRow.ID), "technical", technical)
		return caseRow
	}

	origin := supportIssueOrigin(technical)
	lookup := db.GetIssueBySupportOriginParams{
		WorkspaceID: caseRow.WorkspaceID,
		OriginType:  pgtype.Text{String: origin, Valid: true},
		OriginID:    caseRow.ID,
	}
	issue, err := h.Queries.GetIssueBySupportOrigin(ctx, lookup)
	if errors.Is(err, pgx.ErrNoRows) {
		title := fmt.Sprintf("[%s] %s — atendimento de suporte", caseRow.PublicCode, supportApplicationName(caseRow.AppKey))
		description := fmt.Sprintf("Caso interno %s do Concierge %s. A conversa e os anexos permanecem no atendimento com controle de acesso próprio.", caseRow.PublicCode, supportApplicationName(caseRow.AppKey))
		priority := "low"
		status := "todo"
		if technical {
			title = fmt.Sprintf("INA-%s — investigação técnica", strings.TrimPrefix(caseRow.PublicCode, "SUP-"))
			description = fmt.Sprintf("Origem: %s\nAplicativo: %s\nResumo: %s\n\nCritério de aceite: causa comprovada, correção isolada, testes relevantes aprovados e retorno registrado no caso de origem.", caseRow.PublicCode, supportApplicationName(caseRow.AppKey), strings.TrimSpace(caseRow.ResolutionSummary.String))
			priority = "high"
			status = "backlog"
			if caseRow.RiskLevel.Valid && caseRow.RiskLevel.String == "low" {
				priority = "medium"
			}
		}
		created, createErr := h.IssueService.Create(ctx, service.IssueCreateParams{
			WorkspaceID: caseRow.WorkspaceID,
			Title:       title,
			Description: pgtype.Text{String: description, Valid: true},
			Status:      status,
			Priority:    priority,
			CreatorType: "member",
			CreatorID:   caseRow.ReporterUserID,
			ProjectID:   projectID,
			OriginType:  pgtype.Text{String: origin, Valid: true},
			OriginID:    caseRow.ID,
		}, service.IssueCreateOpts{Platform: "support"})
		if createErr == nil {
			issue = created.Issue
		} else if errors.Is(createErr, service.ErrActiveDuplicate) && created.DuplicateIssue != nil {
			issue = *created.DuplicateIssue
		} else if issue, err = h.Queries.GetIssueBySupportOrigin(ctx, lookup); err != nil {
			slog.Warn("support issue: create failed", "case_id", uuidToString(caseRow.ID), "technical", technical, "error", createErr)
			return caseRow
		}
	} else if err != nil {
		slog.Warn("support issue: lookup failed", "case_id", uuidToString(caseRow.ID), "technical", technical, "error", err)
		return caseRow
	}

	if technical {
		updated, updateErr := h.Queries.SetSupportCaseTechnicalIssue(ctx, db.SetSupportCaseTechnicalIssueParams{
			IssueID: issue.ID, ID: caseRow.ID, WorkspaceID: caseRow.WorkspaceID,
		})
		if updateErr == nil {
			return updated
		}
	} else {
		updated, updateErr := h.Queries.SetSupportCaseSupportIssue(ctx, db.SetSupportCaseSupportIssueParams{
			IssueID: issue.ID, ID: caseRow.ID, WorkspaceID: caseRow.WorkspaceID,
		})
		if updateErr == nil {
			return updated
		}
	}
	slog.Warn("support issue: case link failed", "case_id", uuidToString(caseRow.ID), "technical", technical)
	return caseRow
}

func (h *Handler) syncSupportIssueStatuses(ctx context.Context, caseRow db.SupportCase) {
	supportStatus := "todo"
	technicalStatus := "backlog"
	switch caseRow.State {
	case "em_investigacao_tecnica", "em_correcao":
		supportStatus = "in_progress"
	case "aguardando_aprovacao", "em_validacao", "pronto_para_publicar":
		supportStatus = "in_review"
	case "concluido", "publicado":
		supportStatus = "done"
	case "rejeitado":
		supportStatus = "cancelled"
	case "bloqueado":
		supportStatus = "blocked"
	}
	switch caseRow.State {
	case "em_correcao":
		technicalStatus = "in_progress"
	case "aguardando_aprovacao", "em_validacao", "pronto_para_publicar":
		technicalStatus = "in_review"
	case "publicado":
		technicalStatus = "done"
	case "rejeitado":
		technicalStatus = "cancelled"
	case "bloqueado":
		technicalStatus = "blocked"
	}
	if caseRow.SupportIssueID.Valid {
		if _, err := h.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
			ID: caseRow.SupportIssueID, Status: supportStatus, WorkspaceID: caseRow.WorkspaceID,
		}); err != nil {
			slog.Warn("support issue: status sync failed", "case_id", uuidToString(caseRow.ID), "error", err)
		}
	}
	if caseRow.TechnicalIssueID.Valid {
		if _, err := h.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
			ID: caseRow.TechnicalIssueID, Status: technicalStatus, WorkspaceID: caseRow.WorkspaceID,
		}); err != nil {
			slog.Warn("support technical issue: status sync failed", "case_id", uuidToString(caseRow.ID), "error", err)
		}
	}
}
