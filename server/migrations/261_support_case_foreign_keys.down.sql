ALTER TABLE support_case_sequence
    DROP CONSTRAINT IF EXISTS support_case_sequence_workspace_id_fkey;

ALTER TABLE support_case_transition
    DROP CONSTRAINT IF EXISTS support_case_transition_support_case_id_fkey;

ALTER TABLE support_case
    DROP CONSTRAINT IF EXISTS support_case_technical_issue_id_fkey,
    DROP CONSTRAINT IF EXISTS support_case_support_issue_id_fkey,
    DROP CONSTRAINT IF EXISTS support_case_chat_session_id_fkey,
    DROP CONSTRAINT IF EXISTS support_case_reporter_user_id_fkey,
    DROP CONSTRAINT IF EXISTS support_case_workspace_id_fkey;
