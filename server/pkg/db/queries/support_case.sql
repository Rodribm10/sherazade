-- name: GetSupportCaseByIdempotency :one
SELECT * FROM support_case
WHERE workspace_id = $1 AND reporter_user_id = $2 AND idempotency_key = $3;

-- name: GetSupportCaseForReporter :one
SELECT * FROM support_case
WHERE id = $1 AND workspace_id = $2 AND reporter_user_id = $3;

-- name: GetSupportCaseBySessionForReporter :one
SELECT * FROM support_case
WHERE chat_session_id = $1 AND workspace_id = $2 AND reporter_user_id = $3;

-- name: ListSupportCasesForReporter :many
SELECT * FROM support_case
WHERE workspace_id = $1 AND reporter_user_id = $2
ORDER BY created_at DESC;

-- name: GetSupportCaseInWorkspace :one
SELECT * FROM support_case
WHERE id = $1 AND workspace_id = $2;

-- name: ListSupportCasesForWorkspace :many
SELECT * FROM support_case
WHERE workspace_id = $1
ORDER BY updated_at DESC
LIMIT 200;

-- name: ListSupportCaseTransitions :many
SELECT * FROM support_case_transition
WHERE support_case_id = $1
ORDER BY created_at ASC, id ASC;

-- name: NextSupportCasePublicSequence :one
INSERT INTO support_case_sequence (workspace_id, next_value)
VALUES ($1, 1)
ON CONFLICT (workspace_id) DO UPDATE
SET next_value = support_case_sequence.next_value + 1
RETURNING next_value;

-- name: CreateSupportCase :one
INSERT INTO support_case (
    workspace_id, public_code, reporter_user_id, chat_session_id, pending_message_id,
    app_key, state, idempotency_key
)
VALUES (
    @workspace_id,
    'SUP-' || CASE WHEN char_length(@case_number::text) < 6 THEN lpad(@case_number::text, 6, '0') ELSE @case_number::text END,
    @reporter_user_id,
    @chat_session_id,
    @pending_message_id,
    'inaudit',
    'novo',
    @idempotency_key
)
RETURNING *;

-- name: CreateSupportCaseTransition :one
INSERT INTO support_case_transition (support_case_id, previous_state, new_state, actor_type, actor_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: LockSupportCase :one
SELECT * FROM support_case
WHERE id = $1 AND workspace_id = $2
FOR UPDATE;

-- name: MarkSupportCasePending :one
UPDATE support_case
SET pending_message_id = @pending_message_id,
    state = 'coletando_contexto',
    risk_level = NULL,
    confidence = NULL,
    resolution_type = NULL,
    resolution_summary = NULL,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: MarkSupportCaseAnalyzing :one
UPDATE support_case
SET state = 'em_analise', updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CompleteSupportCaseAnalysis :one
UPDATE support_case
SET state = @state,
    risk_level = @risk_level,
    confidence = @confidence,
    resolution_type = @resolution_type,
    resolution_summary = @resolution_summary,
    last_answered_message_id = @last_answered_message_id,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: SetSupportCaseSupportIssue :one
UPDATE support_case
SET support_issue_id = COALESCE(support_issue_id, @issue_id), updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: SetSupportCaseTechnicalIssue :one
UPDATE support_case
SET technical_issue_id = COALESCE(technical_issue_id, @issue_id), updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: RequestSupportCaseApproval :one
UPDATE support_case
SET state = 'aguardando_aprovacao',
    approval_revision = approval_revision + 1,
    approval_summary = @approval_summary,
    approval_by = NULL,
    approval_at = NULL,
    rejected_at = NULL,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ApproveSupportCaseExecution :one
UPDATE support_case
SET state = 'em_correcao',
    approval_by = @approval_by,
    approval_at = now(),
    rejected_at = NULL,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: RejectSupportCaseExecution :one
UPDATE support_case
SET state = 'rejeitado',
    approval_by = @approval_by,
    rejected_at = now(),
    approval_at = NULL,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CompleteSupportTechnicalWork :one
UPDATE support_case
SET state = @state,
    resolution_summary = @resolution_summary,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: GetSupportCaseMetrics :one
SELECT
    COUNT(*)::bigint AS total_cases,
    COUNT(*) FILTER (WHERE resolution_type = 'answer')::bigint AS automatic_answers,
    COUNT(*) FILTER (WHERE technical_issue_id IS NOT NULL)::bigint AS technical_escalations,
    COUNT(*) FILTER (WHERE confirmed_at IS NOT NULL)::bigint AS confirmed_resolutions,
    COUNT(*) FILTER (WHERE state = 'aguardando_aprovacao')::bigint AS awaiting_approval,
    COUNT(*) FILTER (WHERE state = 'concluido')::bigint AS concluded_cases,
    COUNT(*) FILTER (WHERE state = 'bloqueado')::bigint AS blocked_cases,
    COALESCE(AVG(EXTRACT(EPOCH FROM (answered_message.created_at - support_case.created_at))) FILTER (WHERE answered_message.id IS NOT NULL), 0)::double precision AS average_response_seconds,
    (
      SELECT COUNT(*)::bigint
      FROM support_case_transition transition
      JOIN support_case scoped ON scoped.id = transition.support_case_id
      WHERE scoped.workspace_id = @workspace_id
        AND transition.new_state = 'aguardando_relator'
        AND transition.previous_state IN ('resposta_proposta', 'aguardando_confirmacao', 'concluido')
    ) AS reopened_cases
FROM support_case
LEFT JOIN chat_message answered_message ON answered_message.id = support_case.last_answered_message_id
WHERE support_case.workspace_id = @workspace_id;

-- name: ConfirmSupportCaseResolution :one
UPDATE support_case
SET state = 'concluido',
    confirmed_at = now(),
    resolved_at = now(),
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ReopenSupportCaseForReporter :one
UPDATE support_case
SET state = 'aguardando_relator',
    pending_message_id = NULL,
    confirmed_at = NULL,
    resolved_at = NULL,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ListRecentSupportChatMessages :many
SELECT * FROM (
    SELECT * FROM chat_message
    WHERE chat_session_id = $1
    ORDER BY created_at DESC, id DESC
    LIMIT 20
) recent
ORDER BY created_at ASC, id ASC;

-- name: DeleteSupportCaseDataByChatSession :exec
WITH cases AS MATERIALIZED (
    SELECT support_case.id
    FROM support_case
    WHERE support_case.chat_session_id = $1
), deleted_transitions AS (
    DELETE FROM support_case_transition
    WHERE support_case_transition.support_case_id IN (SELECT cases.id FROM cases)
)
DELETE FROM support_case WHERE support_case.id IN (SELECT cases.id FROM cases);
