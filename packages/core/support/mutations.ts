import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type {
  CreateSupportSessionInput,
  SendSupportMessageInput,
  SupportMessage,
  SupportSession,
} from "../types/support";
import { supportKeys } from "./queries";

export function useCreateSupportSession(wsId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: CreateSupportSessionInput) =>
      api.createSupportSession(input),
    onSuccess: (created) => {
      if (!created.id || !created.session_id) return;
      queryClient.setQueryData<SupportSession[]>(
        supportKeys.sessions(wsId),
        (current = []) => {
          if (current.some((session) => session.id === created.id))
            return current;
          return [created, ...current];
        },
      );
      queryClient.invalidateQueries({
        queryKey: supportKeys.messages(wsId, created.session_id),
      });
    },
  });
}

export function useSendSupportMessage(wsId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ sessionId, content }: SendSupportMessageInput) =>
      api.sendSupportMessage(sessionId, content),
    onSuccess: (message, { sessionId }) => {
      if (!message.id) return;
      queryClient.setQueryData<SupportMessage[]>(
        supportKeys.messages(wsId, sessionId),
        (current = []) => {
          if (current.some((item) => item.id === message.id)) return current;
          return [...current, message];
        },
      );
      queryClient.invalidateQueries({
        queryKey: supportKeys.messages(wsId, sessionId),
      });
      queryClient.invalidateQueries({
        queryKey: supportKeys.sessions(wsId),
      });
    },
  });
}
