-- Add reports_to column to agent: enables an org-chart-style hierarchy
-- where one agent can "report to" a supervisor agent. Nullable by default
-- so existing agents are unchanged. ON DELETE SET NULL so deleting a
-- supervisor gracefully orphans direct reports instead of cascading.
ALTER TABLE agent
  ADD COLUMN reports_to UUID REFERENCES agent(id) ON DELETE SET NULL;

-- Prevent an agent from reporting to itself (single-node cycle).
-- Longer cycles (A -> B -> A) must be enforced in application code
-- because Postgres cannot express multi-row CHECK constraints.
ALTER TABLE agent
  ADD CONSTRAINT agent_reports_to_not_self
  CHECK (reports_to IS NULL OR reports_to <> id);

-- Index supports "list direct reports of X" and "build tree for workspace Y".
CREATE INDEX idx_agent_workspace_reports_to
  ON agent (workspace_id, reports_to);
