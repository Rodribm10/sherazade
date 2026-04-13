"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Bot, MessageSquare, Archive, Trash2 } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@multica/ui/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { ChevronDown } from "lucide-react";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@multica/ui/components/ui/resizable";
import { useDefaultLayout } from "react-resizable-panels";
import { useWorkspaceId } from "@multica/core/hooks";
import { useAuthStore } from "@multica/core/auth";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { canAssignAgent } from "@multica/views/issues/components";
import { api } from "@multica/core/api";
import {
  allChatSessionsOptions,
  chatMessagesOptions,
  chatKeys,
} from "@multica/core/chat/queries";
import { useCreateChatSession, useArchiveChatSession } from "@multica/core/chat/mutations";
import { useChatStore } from "@multica/core/chat";
import { useWS } from "@multica/core/realtime";
import { issueDetailOptions } from "@multica/core/issues/queries";
import { useUpdateIssue } from "@multica/core/issues/mutations";
import { StatusPicker } from "@multica/views/issues/components";
import type {
  TaskMessagePayload,
  ChatDonePayload,
  CommentCreatedPayload,
  Agent,
  ChatMessage,
  ChatSession,
  IssueStatus,
} from "@multica/core/types";
import { ChatMessageList } from "./chat-message-list";
import { ChatInput } from "./chat-input";

/**
 * Full-page chat experience. Reuses the same zustand store the floating
 * ChatWindow uses, so history, streaming, optimistic UI, and cancel all
 * work identically — just in a dedicated route with a persistent session
 * sidebar on the left and the active conversation filling the rest.
 */
export function ChatPage() {
  const wsId = useWorkspaceId();

  const activeSessionId = useChatStore((s) => s.activeSessionId);
  const pendingTaskId = useChatStore((s) => s.pendingTaskId);
  const timelineItems = useChatStore((s) => s.timelineItems);
  const selectedAgentId = useChatStore((s) => s.selectedAgentId);
  const setActiveSession = useChatStore((s) => s.setActiveSession);
  const setPendingTask = useChatStore((s) => s.setPendingTask);
  const addTimelineItem = useChatStore((s) => s.addTimelineItem);
  const clearTimeline = useChatStore((s) => s.clearTimeline);
  const setSelectedAgentId = useChatStore((s) => s.setSelectedAgentId);

  const user = useAuthStore((s) => s.user);
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: allSessions = [] } = useQuery(allChatSessionsOptions(wsId));
  const { data: rawMessages } = useQuery(
    chatMessagesOptions(activeSessionId ?? ""),
  );
  const messages = activeSessionId ? rawMessages ?? [] : [];

  const currentMember = members.find((m) => m.user_id === user?.id);
  const memberRole = currentMember?.role;
  const availableAgents = useMemo(
    () =>
      agents.filter(
        (a) => !a.archived_at && canAssignAgent(a, user?.id, memberRole),
      ),
    [agents, user?.id, memberRole],
  );
  const activeAgent =
    availableAgents.find((a) => a.id === selectedAgentId) ??
    availableAgents[0] ??
    null;

  const currentSession = activeSessionId
    ? allSessions.find((s) => s.id === activeSessionId)
    : null;
  const isSessionArchived = currentSession?.status === "archived";
  const linkedIssueId = currentSession?.issue_id ?? null;

  // Chat-first sessions are backed by a real issue. Fetch it so we can
  // render the status picker in the header and let the user drive the
  // Kanban manually from inside the chat.
  const { data: linkedIssue } = useQuery({
    ...issueDetailOptions(wsId, linkedIssueId ?? ""),
    enabled: !!linkedIssueId,
  });
  const updateIssue = useUpdateIssue();

  const qc = useQueryClient();
  const createSession = useCreateChatSession();
  const archiveSession = useArchiveChatSession();

  const { defaultLayout, onLayoutChanged } = useDefaultLayout({
    id: "multica_chat_layout",
  });

  // Keep pendingTaskId in a ref so WS handlers always see the latest value.
  const pendingTaskRef = useRef<string | null>(pendingTaskId);
  pendingTaskRef.current = pendingTaskId;

  const { subscribe } = useWS();

  useEffect(() => {
    const matchesPending = (taskId: string) =>
      !!pendingTaskRef.current && taskId === pendingTaskRef.current;

    const finalizePending = (invalidateCache: boolean) => {
      if (invalidateCache) {
        const sid = useChatStore.getState().activeSessionId;
        if (sid) {
          qc.invalidateQueries({ queryKey: chatKeys.messages(sid) });
        }
      }
      clearTimeline();
      setPendingTask(null);
    };

    const unsubMessage = subscribe("task:message", (payload) => {
      const p = payload as TaskMessagePayload;
      if (!matchesPending(p.task_id)) return;
      addTimelineItem({
        seq: p.seq,
        type: p.type,
        tool: p.tool,
        content: p.content,
        input: p.input,
        output: p.output,
      });
    });

    const unsubDone = subscribe("chat:done", (payload) => {
      const p = payload as ChatDonePayload;
      if (!matchesPending(p.task_id)) return;
      finalizePending(true);
    });

    const unsubCompleted = subscribe("task:completed", (payload) => {
      const p = payload as { task_id: string };
      if (!matchesPending(p.task_id)) return;
      finalizePending(true);
    });

    const unsubFailed = subscribe("task:failed", (payload) => {
      const p = payload as { task_id: string };
      if (!matchesPending(p.task_id)) return;
      finalizePending(false);
    });

    // Chat-first sessions render issue comments. When a new comment
    // lands (agent reply, mention cascade, etc.), refresh the message
    // list so the user sees it without reloading.
    const unsubComment = subscribe("comment:created", (payload) => {
      const p = payload as CommentCreatedPayload;
      const sid = useChatStore.getState().activeSessionId;
      if (!sid) return;
      const session = qc
        .getQueryData<ChatSession[]>(chatKeys.allSessions(wsId))
        ?.find((s) => s.id === sid);
      if (!session?.issue_id) return;
      if (p.comment.issue_id !== session.issue_id) return;
      qc.invalidateQueries({ queryKey: chatKeys.messages(sid) });
    });

    return () => {
      unsubMessage();
      unsubDone();
      unsubCompleted();
      unsubFailed();
      unsubComment();
    };
  }, [subscribe, addTimelineItem, clearTimeline, setPendingTask, qc, wsId]);

  const handleSend = useCallback(
    async (content: string) => {
      if (!activeAgent) return;
      let sessionId = activeSessionId;

      // Chat-first flow: the very first message of a new chat creates
      // an issue + session in one call, and the backend enqueues the
      // initial task from the issue description. No separate
      // sendChatMessage call needed here — the prompt IS the task.
      if (!sessionId) {
        const session = await createSession.mutateAsync({
          agent_id: activeAgent.id,
          title: content.slice(0, 80),
          create_issue: true,
          prompt: content,
        });
        sessionId = session.id;
        setActiveSession(sessionId);
        if (session.initial_task_id) {
          setPendingTask(session.initial_task_id);
        }
        qc.invalidateQueries({ queryKey: chatKeys.messages(sessionId) });
        return;
      }

      const optimistic: ChatMessage = {
        id: `optimistic-${Date.now()}`,
        chat_session_id: sessionId,
        role: "user",
        content,
        task_id: null,
        created_at: new Date().toISOString(),
      };
      qc.setQueryData<ChatMessage[]>(
        chatKeys.messages(sessionId),
        (old) => (old ? [...old, optimistic] : [optimistic]),
      );

      const result = await api.sendChatMessage(sessionId, content);
      setPendingTask(result.task_id);
      qc.invalidateQueries({ queryKey: chatKeys.messages(sessionId) });
    },
    [activeSessionId, activeAgent, createSession, setActiveSession, setPendingTask, qc],
  );

  const handleStop = useCallback(async () => {
    if (!pendingTaskId) return;
    try {
      await api.cancelTaskById(pendingTaskId);
    } catch {
      // Already completed — silent.
    }
    if (activeSessionId) {
      qc.invalidateQueries({ queryKey: chatKeys.messages(activeSessionId) });
    }
    clearTimeline();
    setPendingTask(null);
  }, [pendingTaskId, activeSessionId, clearTimeline, setPendingTask, qc]);

  const handleNewChat = () => {
    setActiveSession(null);
    clearTimeline();
    setPendingTask(null);
  };

  const handleSelectSession = (session: ChatSession) => {
    setActiveSession(session.id);
    clearTimeline();
    setPendingTask(null);
  };

  const handleArchiveSession = (e: React.MouseEvent, sessionId: string) => {
    e.stopPropagation();
    archiveSession.mutate(sessionId);
    if (activeSessionId === sessionId) {
      setActiveSession(null);
    }
  };

  const handleSelectAgent = (agent: Agent) => {
    setSelectedAgentId(agent.id);
    setActiveSession(null);
    clearTimeline();
    setPendingTask(null);
  };

  const activeSessionList = allSessions.filter((s) => s.status === "active");
  const archivedSessionList = allSessions.filter((s) => s.status === "archived");
  const hasMessages = messages.length > 0 || timelineItems.length > 0;
  const agentMap = new Map(agents.map((a) => [a.id, a]));

  return (
    <ResizablePanelGroup
      orientation="horizontal"
      className="flex-1 min-h-0"
      defaultLayout={defaultLayout}
      onLayoutChanged={onLayoutChanged}
    >
      {/* Left column — sessions */}
      <ResizablePanel
        id="sessions"
        defaultSize={280}
        minSize={220}
        maxSize={400}
        groupResizeBehavior="preserve-pixel-size"
      >
        <div className="flex h-full flex-col overflow-hidden border-r">
          <div className="flex h-12 items-center justify-between border-b px-4">
            <h1 className="text-sm font-semibold">Chat</h1>
            <button
              type="button"
              onClick={handleNewChat}
              title="New chat"
              className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              <Plus className="size-4" />
            </button>
          </div>
          <div className="flex-1 overflow-y-auto">
            {allSessions.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground">
                <MessageSquare className="size-6" />
                <span className="text-sm">No chat sessions yet</span>
              </div>
            ) : (
              <>
                {activeSessionList.length > 0 && (
                  <SessionGroup
                    label="Active"
                    sessions={activeSessionList}
                    agentMap={agentMap}
                    activeSessionId={activeSessionId}
                    onSelect={handleSelectSession}
                    onArchive={handleArchiveSession}
                  />
                )}
                {archivedSessionList.length > 0 && (
                  <SessionGroup
                    label="Archived"
                    sessions={archivedSessionList}
                    agentMap={agentMap}
                    activeSessionId={activeSessionId}
                    onSelect={handleSelectSession}
                  />
                )}
              </>
            )}
          </div>
        </div>
      </ResizablePanel>

      <ResizableHandle />

      {/* Right column — active conversation */}
      <ResizablePanel id="conversation" minSize="40%">
        <div className="flex h-full flex-col">
          <div className="flex h-12 items-center justify-between border-b px-6">
            <AgentSelector
              agents={availableAgents}
              activeAgent={activeAgent}
              onSelect={handleSelectAgent}
            />
            <div className="flex items-center gap-3 min-w-0">
              {currentSession && (
                <span className="truncate text-xs text-muted-foreground">
                  {currentSession.title || "Untitled"}
                </span>
              )}
              {linkedIssue && (
                <StatusPicker
                  status={linkedIssue.status as IssueStatus}
                  onUpdate={(updates) => {
                    if (!updates.status) return;
                    updateIssue.mutate({ id: linkedIssue.id, status: updates.status });
                  }}
                />
              )}
            </div>
          </div>
          {hasMessages ? (
            <ChatMessageList
              messages={messages}
              agent={activeAgent}
              timelineItems={timelineItems}
              isWaiting={!!pendingTaskId}
            />
          ) : (
            <EmptyState agentName={activeAgent?.name} />
          )}
          <ChatInput
            onSend={handleSend}
            onStop={handleStop}
            isRunning={!!pendingTaskId}
            disabled={isSessionArchived}
          />
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}

// ─── Sidebar subcomponents ──────────────────────────────────────────────

function SessionGroup({
  label,
  sessions,
  agentMap,
  activeSessionId,
  onSelect,
  onArchive,
}: {
  label: string;
  sessions: ChatSession[];
  agentMap: Map<string, Agent>;
  activeSessionId: string | null;
  onSelect: (session: ChatSession) => void;
  onArchive?: (e: React.MouseEvent, sessionId: string) => void;
}) {
  return (
    <div>
      <div className="px-4 pt-3 pb-1">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {label}
        </span>
      </div>
      {sessions.map((session) => (
        <SessionItem
          key={session.id}
          session={session}
          agent={agentMap.get(session.agent_id) ?? null}
          isActive={session.id === activeSessionId}
          onSelect={() => onSelect(session)}
          onArchive={onArchive ? (e) => onArchive(e, session.id) : undefined}
        />
      ))}
    </div>
  );
}

function SessionItem({
  session,
  agent,
  isActive,
  onSelect,
  onArchive,
}: {
  session: ChatSession;
  agent: Agent | null;
  isActive: boolean;
  onSelect: () => void;
  onArchive?: (e: React.MouseEvent) => void;
}) {
  const timeAgo = formatTimeAgo(session.updated_at);

  return (
    <button
      onClick={onSelect}
      className={`group flex w-full items-start gap-3 px-4 py-2.5 text-left transition-colors hover:bg-accent/50 ${
        isActive ? "bg-accent/30" : ""
      }`}
    >
      <Avatar className="size-6 shrink-0 mt-0.5">
        {agent?.avatar_url && <AvatarImage src={agent.avatar_url} />}
        <AvatarFallback className="bg-purple-100 text-purple-700 text-[10px]">
          <Bot className="size-3" />
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">
            {session.title || "Untitled"}
          </span>
          {session.status === "archived" && (
            <Archive className="size-3 shrink-0 text-muted-foreground" />
          )}
        </div>
        <div className="flex items-center gap-1.5 mt-0.5">
          {agent && (
            <span className="text-xs text-muted-foreground truncate">
              {agent.name}
            </span>
          )}
          <span className="text-xs text-muted-foreground/60">{timeAgo}</span>
        </div>
      </div>
      {onArchive && (
        <button
          onClick={onArchive}
          title="Archive"
          className="invisible group-hover:visible flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-destructive shrink-0 mt-0.5"
        >
          <Trash2 className="size-3" />
        </button>
      )}
    </button>
  );
}

// ─── Right-column subcomponents ─────────────────────────────────────────

function AgentSelector({
  agents,
  activeAgent,
  onSelect,
}: {
  agents: Agent[];
  activeAgent: Agent | null;
  onSelect: (agent: Agent) => void;
}) {
  if (!activeAgent) {
    return <span className="text-sm text-muted-foreground">No agents</span>;
  }
  if (agents.length <= 1) {
    return (
      <div className="flex items-center gap-2">
        <AgentAvatarSmall agent={activeAgent} />
        <span className="text-sm font-medium">{activeAgent.name}</span>
      </div>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 rounded-md px-1.5 py-1 -ml-1.5 transition-colors hover:bg-accent">
        <AgentAvatarSmall agent={activeAgent} />
        <span className="text-sm font-medium">{activeAgent.name}</span>
        <ChevronDown className="size-3 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {agents.map((agent) => (
          <DropdownMenuItem
            key={agent.id}
            onClick={() => onSelect(agent)}
            className="flex items-center gap-2"
          >
            <AgentAvatarSmall agent={agent} />
            <span>{agent.name}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AgentAvatarSmall({ agent }: { agent: Agent }) {
  return (
    <Avatar className="size-5">
      {agent.avatar_url && <AvatarImage src={agent.avatar_url} />}
      <AvatarFallback className="bg-purple-100 text-purple-700 text-[10px]">
        <Bot className="size-3" />
      </AvatarFallback>
    </Avatar>
  );
}

function EmptyState({ agentName }: { agentName?: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 px-8">
      <MessageSquare className="size-10 text-muted-foreground/30" />
      <div className="text-center">
        <h3 className="text-base font-semibold">
          {agentName ? `Chat with ${agentName}` : "Start a new chat"}
        </h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Ask anything — the agent has your workspace context and can run
          commands, read files, and post back results live.
        </p>
      </div>
    </div>
  );
}

function formatTimeAgo(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}
