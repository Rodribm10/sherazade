ALTER TABLE agent DROP CONSTRAINT IF EXISTS agent_spent_non_negative;
ALTER TABLE agent DROP CONSTRAINT IF EXISTS agent_budget_non_negative;
ALTER TABLE agent
  DROP COLUMN IF EXISTS budget_period_start,
  DROP COLUMN IF EXISTS spent_monthly_cents,
  DROP COLUMN IF EXISTS budget_monthly_cents;
