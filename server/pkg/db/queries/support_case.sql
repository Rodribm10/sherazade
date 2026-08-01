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
