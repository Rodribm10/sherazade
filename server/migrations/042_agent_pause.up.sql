-- Per-agent pause state. Inspired by Paperclip's pauseReason/pausedAt.
--
-- paused_at: timestamp when the pause took effect. NULL means active.
-- pause_reason: optional free-text explanation (e.g. "too expensive",
--   "waiting on oncall approval", "broken, don't run"). Shown in the
--   UI so humans know why the agent is sitting out.
--
-- When paused_at IS NOT NULL, the task claim loop skips this agent
-- entirely. Tasks stay queued until the agent is resumed. This is
-- distinct from archive (which hides the agent and cancels running
-- tasks): pause keeps the agent fully visible and preserves history.
ALTER TABLE agent
  ADD COLUMN paused_at TIMESTAMPTZ,
  ADD COLUMN pause_reason TEXT;
