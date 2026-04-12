-- Scheduled tasks: cron-like recurring prompts that spawn a new issue
-- each time they fire. Inspired by "every Monday 9am, ask Dev Backend
-- to check failing tests" style automations.
--
-- Execution model (lazy, no background job per schedule):
--   - A single server-side ticker runs every 60s
--   - It picks schedules where enabled = true AND next_run_at <= now()
--   - For each due schedule: it creates a new issue with title/prompt
--     assigned to the agent, then advances next_run_at via cron parsing
--
-- cron_expr is a standard 5-field expression (robfig/cron v3 standard
-- syntax). Parsing happens in Go — no pg_cron extension needed.
CREATE TABLE scheduled_task (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  created_by UUID NOT NULL REFERENCES "user"(id),
  title TEXT NOT NULL,
  prompt TEXT NOT NULL,
  cron_expr TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial index: the ticker only cares about enabled schedules.
CREATE INDEX idx_scheduled_task_due
  ON scheduled_task (next_run_at)
  WHERE enabled = true;

CREATE INDEX idx_scheduled_task_workspace
  ON scheduled_task (workspace_id);
