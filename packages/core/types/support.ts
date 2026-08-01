export interface SupportSession {
  id: string;
  public_code: string;
  session_id: string;
  app_key: string;
  state: string;
}

export interface SupportMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  created_at: string;
}

export interface CreateSupportSessionInput {
  idempotency_key: string;
  description: string;
}

export interface SendSupportMessageInput {
  sessionId: string;
  content: string;
}
