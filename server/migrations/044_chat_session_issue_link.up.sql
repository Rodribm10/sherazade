-- Link chat_session to an optional issue so chats created from the
-- "New chat" dialog become real, Kanban-visible issues without losing
-- the chat UX. Legacy sessions keep issue_id NULL and continue using
-- chat_message storage; new sessions created via the chat→issue flow
-- set issue_id and route messages through issue comments instead.
--
-- ON DELETE SET NULL so deleting the issue doesn't orphan the session
-- row — the chat history survives in chat_message even if the issue
-- is wiped.
ALTER TABLE chat_session
  ADD COLUMN issue_id UUID REFERENCES issue(id) ON DELETE SET NULL;

-- Index for the reverse lookup: "is this issue chat-driven?". Used by
-- the claim path to detect when it should tell the agent not to touch
-- issue status manually (the human drives the Kanban).
CREATE INDEX idx_chat_session_issue_id
  ON chat_session (issue_id)
  WHERE issue_id IS NOT NULL;
