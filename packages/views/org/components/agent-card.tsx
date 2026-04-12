"use client";

import { useQuery } from "@tanstack/react-query";
import type { Agent } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { runtimeListOptions } from "@multica/core/runtimes/queries";
import { ActorAvatar } from "../../common/actor-avatar";
import { statusConfig } from "../../agents/config";
import { AppLink } from "../../navigation";

/**
 * Compact card rendered inside the org chart for a single agent.
 * Visual hierarchy: avatar · name · description · provider. Clicking
 * the card navigates to /agents with the detail pane open on that
 * agent (same surface the org chart pulls data from).
 */
export function AgentCard({ agent }: { agent: Agent }) {
  const wsId = useWorkspaceId();
  const { data: runtimes = [] } = useQuery(runtimeListOptions(wsId));
  const runtime = runtimes.find((r) => r.id === agent.runtime_id);
  const providerLabel = runtime?.provider ?? agent.runtime_mode;

  const status = statusConfig[agent.status];

  return (
    <AppLink
      href={`/agents?selected=${agent.id}`}
      className="group relative block w-56 rounded-lg border border-border bg-card px-4 py-3 shadow-sm transition-colors hover:border-primary/50 hover:shadow-md"
    >
      <div className="flex items-start gap-3">
        <ActorAvatar
          actorType="agent"
          actorId={agent.id}
          size={36}
          className="shrink-0"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold">{agent.name}</span>
            <span
              className={`size-1.5 shrink-0 rounded-full ${status.dot}`}
              title={status.label}
              aria-label={status.label}
            />
          </div>
          {agent.description && (
            <div className="mt-0.5 truncate text-xs text-muted-foreground">
              {agent.description}
            </div>
          )}
          <div className="mt-1 truncate text-[10px] uppercase tracking-wider text-muted-foreground/70">
            {providerLabel}
          </div>
        </div>
      </div>
    </AppLink>
  );
}
