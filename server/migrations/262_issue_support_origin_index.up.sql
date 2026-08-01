CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS issue_support_origin_unique
ON issue (workspace_id, origin_type, origin_id)
WHERE origin_type IN ('support_case', 'support_technical') AND origin_id IS NOT NULL;
