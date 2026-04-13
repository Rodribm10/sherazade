export interface ChatSession {
  id: string;
  workspace_id: string;
  agent_id: string;
  creator_id: string;
  title: string;
  status: "active" | "archived";
  issue_id: string | null;
  issue_status?: string;
  created_at: string;
  updated_at: string;
  // Populated only in the chat-first create response, so the caller can
  // show a running indicator until the first agent reply lands.
  initial_task_id?: string;
}

export interface ChatMessage {
  id: string;
  chat_session_id: string;
  role: "user" | "assistant";
  content: string;
  task_id: string | null;
  created_at: string;
}

export interface SendChatMessageResponse {
  message_id: string;
  task_id: string;
}
