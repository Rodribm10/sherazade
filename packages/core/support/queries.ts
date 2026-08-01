import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const supportKeys = {
  all: (wsId: string) => ["support", wsId] as const,
  sessions: (wsId: string) => [...supportKeys.all(wsId), "sessions"] as const,
  messages: (wsId: string, sessionId: string) =>
    [...supportKeys.all(wsId), "sessions", sessionId, "messages"] as const,
};

export function supportSessionsOptions(wsId: string) {
  return queryOptions({
    queryKey: supportKeys.sessions(wsId),
    queryFn: () => api.listSupportSessions(),
    enabled: wsId.length > 0,
  });
}

export function supportMessagesOptions(wsId: string, sessionId: string) {
  return queryOptions({
    queryKey: supportKeys.messages(wsId, sessionId),
    queryFn: () => api.listSupportMessages(sessionId),
    enabled: wsId.length > 0 && sessionId.length > 0,
  });
}
