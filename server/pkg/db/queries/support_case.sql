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
    workspace_id, public_code, reporter_user_id, chat_session_id, app_key, state, idempotency_key
)
VALUES (@workspace_id, 'SUP-' || CASE WHEN char_length(@case_number::text) < 6 THEN lpad(@case_number::text, 6, '0') ELSE @case_number::text END, @reporter_user_id, @chat_session_id, 'inaudit', 'novo', @idempotency_key)
RETURNING *;

-- name: CreateSupportCaseTransition :one
INSERT INTO support_case_transition (support_case_id, previous_state, new_state, actor_user_id)
VALUES ($1, $2, $3, $4)
RETURNING *;
