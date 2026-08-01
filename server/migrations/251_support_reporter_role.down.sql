DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM member WHERE role = 'reporter')
       OR EXISTS (SELECT 1 FROM workspace_invitation WHERE role = 'reporter') THEN
        RAISE EXCEPTION 'cannot roll back support reporter role while reporter members or invitations exist';
    END IF;
END $$;

ALTER TABLE member DROP CONSTRAINT IF EXISTS member_role_check;
ALTER TABLE member
    ADD CONSTRAINT member_role_check CHECK (role IN ('owner', 'admin', 'member'));

ALTER TABLE workspace_invitation DROP CONSTRAINT IF EXISTS workspace_invitation_role_check;
ALTER TABLE workspace_invitation
    ADD CONSTRAINT workspace_invitation_role_check CHECK (role IN ('admin', 'member'));
