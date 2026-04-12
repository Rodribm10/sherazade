-- name: CreateScheduledTask :one
INSERT INTO scheduled_task (
    workspace_id, agent_id, created_by, title, prompt, cron_expr, next_run_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetScheduledTask :one
SELECT * FROM scheduled_task WHERE id = $1;

-- name: GetScheduledTaskInWorkspace :one
SELECT * FROM scheduled_task WHERE id = $1 AND workspace_id = $2;

-- name: ListScheduledTasks :many
SELECT * FROM scheduled_task
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: UpdateScheduledTask :one
UPDATE scheduled_task SET
    title = COALESCE(sqlc.narg('title'), title),
    prompt = COALESCE(sqlc.narg('prompt'), prompt),
    cron_expr = COALESCE(sqlc.narg('cron_expr'), cron_expr),
    next_run_at = COALESCE(sqlc.narg('next_run_at'), next_run_at),
    agent_id = COALESCE(sqlc.narg('agent_id'), agent_id),
    enabled = COALESCE(sqlc.narg('enabled'), enabled),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteScheduledTask :exec
DELETE FROM scheduled_task WHERE id = $1;

-- name: MarkScheduledTaskRun :one
-- Called by the ticker after successfully enqueueing an issue for this
-- schedule. Sets last_run_at = now() and advances next_run_at to the
-- value computed from the cron expression.
UPDATE scheduled_task SET
    last_run_at = now(),
    next_run_at = sqlc.arg('next_run_at')::timestamptz,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListDueScheduledTasks :many
-- Returns enabled schedules whose next_run_at has passed. Used by the
-- server-side ticker to decide which schedules to fire right now.
SELECT * FROM scheduled_task
WHERE enabled = true AND next_run_at <= now()
ORDER BY next_run_at ASC
LIMIT 100;
