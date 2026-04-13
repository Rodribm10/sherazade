import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { chatKeys } from "./queries";

export function useCreateChatSession() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (data: {
      agent_id: string;
      title?: string;
      create_issue?: boolean;
      prompt?: string;
    }) => api.createChatSession(data),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
      qc.invalidateQueries({ queryKey: chatKeys.allSessions(wsId) });
      // Kanban and issue list — chat-first flow creates an issue.
      qc.invalidateQueries({ queryKey: ["issues"] });
    },
  });
}

export function useArchiveChatSession() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (sessionId: string) => api.archiveChatSession(sessionId),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
      qc.invalidateQueries({ queryKey: chatKeys.allSessions(wsId) });
    },
  });
}
