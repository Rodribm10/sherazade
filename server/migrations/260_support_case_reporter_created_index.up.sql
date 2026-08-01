CREATE INDEX CONCURRENTLY support_case_reporter_created_idx ON support_case (workspace_id, reporter_user_id, created_at DESC);
