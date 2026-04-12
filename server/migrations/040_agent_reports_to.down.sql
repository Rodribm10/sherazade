DROP INDEX IF EXISTS idx_agent_workspace_reports_to;
ALTER TABLE agent DROP CONSTRAINT IF EXISTS agent_reports_to_not_self;
ALTER TABLE agent DROP COLUMN IF EXISTS reports_to;
