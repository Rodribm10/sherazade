CREATE UNIQUE INDEX CONCURRENTLY support_case_idempotency_unique ON support_case (workspace_id, reporter_user_id, idempotency_key);
