import type { Attachment } from "./attachment";

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
  attachments?: Attachment[];
}

export interface CreateSupportSessionInput {
  idempotency_key: string;
  description: string;
  defer_analysis?: boolean;
}

export interface SendSupportMessageInput {
  sessionId: string;
  content: string;
  attachmentIds?: string[];
}

export interface SupportAdminCase {
  id: string;
  public_code: string;
  reporter_user_id: string;
  session_id: string;
  support_issue_id: string | null;
  technical_issue_id: string | null;
  app_key: string;
  state: string;
  risk_level: string | null;
  confidence: string | null;
  resolution_type: string | null;
  resolution_summary: string | null;
  approval_revision: number;
  approval_summary: string | null;
  approval_by: string | null;
  approval_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface SupportCaseTransition {
  previous_state: string | null;
  new_state: string;
  actor_type: string;
  actor_id: string;
  created_at: string;
}

export interface SupportAdminDetail {
  case: SupportAdminCase;
  messages: SupportMessage[];
  transitions: SupportCaseTransition[];
}

export interface SupportMetrics {
  total_cases: number;
  automatic_answers: number;
  technical_escalations: number;
  confirmed_resolutions: number;
  awaiting_approval: number;
  concluded_cases: number;
  blocked_cases: number;
  average_response_seconds: number;
  reopened_cases: number;
}
