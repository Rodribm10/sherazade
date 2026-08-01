ALTER TABLE support_case
    ADD COLUMN approval_revision INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN approval_summary TEXT,
    ADD COLUMN approval_by UUID,
    ADD COLUMN approval_at TIMESTAMPTZ,
    ADD COLUMN rejected_at TIMESTAMPTZ;

ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_origin_type_check;
ALTER TABLE issue ADD CONSTRAINT issue_origin_type_check
    CHECK (origin_type IN ('autopilot', 'quick_create', 'lark_chat', 'slack_chat', 'agent_create', 'support_case', 'support_technical'));
