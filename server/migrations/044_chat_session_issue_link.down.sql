DROP INDEX IF EXISTS idx_chat_session_issue_id;
ALTER TABLE chat_session DROP COLUMN IF EXISTS issue_id;
