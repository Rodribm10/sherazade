"use client";

import { useMemo } from "react";
import { ChevronRight } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { ActorAvatar } from "../../common/actor-avatar";
import { statusConfig } from "../config";

/**
 * Renders a list of agents as an org chart, grouped by reports_to.
 *
 * - Roots: agents whose reports_to is null (or points to an agent not in
 *   the provided list — defensive handling for dangling references).
 * - Children are nested recursively with indentation and a chevron icon.
 * - A cycle guard prevents infinite loops if the data is ever corrupted.
 * - Archived agents are included only if they appear in the input; the
 *   caller decides whether to filter them out.
 */
export function AgentOrgTree({
  agents,
  selectedId,
  onSelect,
}: {
  agents: Agent[];
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  // Build an adjacency map: supervisor_id -> direct reports.
  const { roots, childrenOf } = useMemo(() => {
    const byId = new Map(agents.map((a) => [a.id, a]));
    const children = new Map<string, Agent[]>();
    const rootList: Agent[] = [];
    for (const a of agents) {
      if (a.reports_to && byId.has(a.reports_to)) {
        const list = children.get(a.reports_to) ?? [];
        list.push(a);
        children.set(a.reports_to, list);
      } else {
        rootList.push(a);
      }
    }
    // Stable ordering: alphabetical within each level.
    rootList.sort((a, b) => a.name.localeCompare(b.name));
    for (const list of children.values()) {
      list.sort((a, b) => a.name.localeCompare(b.name));
    }
    return { roots: rootList, childrenOf: children };
  }, [agents]);

  if (agents.length === 0) {
    return null;
  }

  return (
    <div className="py-2">
      {roots.map((root) => (
        <TreeNode
          key={root.id}
          agent={root}
          depth={0}
          childrenOf={childrenOf}
          visited={new Set()}
          selectedId={selectedId}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

function TreeNode({
  agent,
  depth,
  childrenOf,
  visited,
  selectedId,
  onSelect,
}: {
  agent: Agent;
  depth: number;
  childrenOf: Map<string, Agent[]>;
  visited: Set<string>;
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  // Cycle guard: if we've already rendered this agent in the current
  // ancestor chain, stop. Shouldn't happen (server prevents cycles) but
  // corrupted data shouldn't freeze the UI.
  if (visited.has(agent.id)) {
    return null;
  }
  const nextVisited = new Set(visited);
  nextVisited.add(agent.id);

  const children = childrenOf.get(agent.id) ?? [];
  const isSelected = agent.id === selectedId;
  const st = statusConfig[agent.status];

  return (
    <>
      <button
        type="button"
        onClick={() => onSelect(agent.id)}
        className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors ${
          isSelected ? "bg-accent" : "hover:bg-accent/50"
        }`}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
      >
        {depth > 0 && (
          <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground/60" />
        )}
        <ActorAvatar
          actorType="agent"
          actorId={agent.id}
          size={24}
          className="shrink-0"
        />
        <span className="min-w-0 flex-1 truncate font-medium">
          {agent.name}
        </span>
        <span
          className={`h-2 w-2 shrink-0 rounded-full ${st.dot}`}
          title={st.label}
        />
      </button>
      {children.map((child) => (
        <TreeNode
          key={child.id}
          agent={child}
          depth={depth + 1}
          childrenOf={childrenOf}
          visited={nextVisited}
          selectedId={selectedId}
          onSelect={onSelect}
        />
      ))}
    </>
  );
}
