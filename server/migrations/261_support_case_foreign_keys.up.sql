ALTER TABLE support_case
    ADD CONSTRAINT support_case_workspace_id_fkey
        FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE,
    ADD CONSTRAINT support_case_reporter_user_id_fkey
        FOREIGN KEY (reporter_user_id) REFERENCES "user"(id) ON DELETE CASCADE,
    ADD CONSTRAINT support_case_chat_session_id_fkey
        FOREIGN KEY (chat_session_id) REFERENCES chat_session(id) ON DELETE CASCADE,
    ADD CONSTRAINT support_case_support_issue_id_fkey
        FOREIGN KEY (support_issue_id) REFERENCES issue(id) ON DELETE SET NULL,
    ADD CONSTRAINT support_case_technical_issue_id_fkey
        FOREIGN KEY (technical_issue_id) REFERENCES issue(id) ON DELETE SET NULL;

ALTER TABLE support_case_transition
    ADD CONSTRAINT support_case_transition_support_case_id_fkey
        FOREIGN KEY (support_case_id) REFERENCES support_case(id) ON DELETE CASCADE;

ALTER TABLE support_case_sequence
    ADD CONSTRAINT support_case_sequence_workspace_id_fkey
        FOREIGN KEY (workspace_id) REFERENCES workspace(id) ON DELETE CASCADE;
