"use client";

import { useMemo } from "react";
import { Bot } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { agentListOptions } from "@multica/core/workspace/queries";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { OrgChart } from "./org-chart";

/**
 * Full-page org chart view. Loads all agents for the current workspace
 * and renders them as a hierarchical card tree driven by the
 * `reports_to` field. Archived agents are filtered out.
 */
export function OrgChartPage() {
  const isLoading = useAuthStore((s) => s.isLoading);
  const wsId = useWorkspaceId();
  const { data: agents = [], isLoading: agentsLoading } = useQuery(agentListOptions(wsId));

  const activeAgents = useMemo(
    () => agents.filter((a) => !a.archived_at),
    [agents],
  );

  if (isLoading || agentsLoading) {
    return (
      <div className="flex h-full flex-col p-8">
        <Skeleton className="h-8 w-48" />
        <div className="mt-8 flex flex-wrap gap-6">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-48 rounded-lg" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="flex h-12 items-center border-b px-6">
        <h1 className="text-sm font-semibold">Org Chart</h1>
        <span className="ml-3 text-xs text-muted-foreground">
          {activeAgents.length} {activeAgents.length === 1 ? "agent" : "agents"}
        </span>
      </header>

      {activeAgents.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="flex-1 overflow-auto">
          <div className="min-w-max p-10">
            <OrgChart agents={activeAgents} />
          </div>
        </div>
      )}
    </div>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center text-muted-foreground">
      <Bot className="h-12 w-12 text-muted-foreground/30" />
      <p className="mt-4 text-sm">No agents yet.</p>
      <p className="mt-1 text-xs text-muted-foreground/70">
        Create agents from the Agents page and set their supervisor to see
        them grouped here.
      </p>
    </div>
  );
}
