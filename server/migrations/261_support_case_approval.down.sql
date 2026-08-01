ALTER TABLE support_case
    DROP COLUMN IF EXISTS rejected_at,
    DROP COLUMN IF EXISTS approval_at,
    DROP COLUMN IF EXISTS approval_by,
    DROP COLUMN IF EXISTS approval_summary,
    DROP COLUMN IF EXISTS approval_revision;

-- Existing support-origin issues must be removed or relabeled before rollback.
ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_origin_type_check;
ALTER TABLE issue ADD CONSTRAINT issue_origin_type_check
    CHECK (origin_type IN ('autopilot', 'quick_create', 'lark_chat', 'slack_chat', 'agent_create'));
