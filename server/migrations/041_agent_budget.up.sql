-- Per-agent monthly budget in cents (USD). Inspired by Paperclip.
--
-- budget_monthly_cents: NULL means unlimited. Otherwise, when an agent's
--   spent_monthly_cents reaches this value in the current period, the task
--   queue refuses to dispatch new tasks to it until rollover.
-- spent_monthly_cents: running total for the current period. Incremented by
--   the usage-reporting endpoint using a model price table.
-- budget_period_start: first day of the current billing month. When a usage
--   report arrives and the current month is later, the server resets spent
--   to 0 and bumps this to the new month (lazy rollover — no cron needed).
ALTER TABLE agent
  ADD COLUMN budget_monthly_cents BIGINT,
  ADD COLUMN spent_monthly_cents BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN budget_period_start DATE NOT NULL DEFAULT date_trunc('month', now())::date;

-- Budget must be non-negative when set; spent is always non-negative.
ALTER TABLE agent
  ADD CONSTRAINT agent_budget_non_negative
  CHECK (budget_monthly_cents IS NULL OR budget_monthly_cents >= 0);

ALTER TABLE agent
  ADD CONSTRAINT agent_spent_non_negative
  CHECK (spent_monthly_cents >= 0);
