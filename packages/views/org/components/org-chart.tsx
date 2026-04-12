"use client";

import { useMemo } from "react";
import { cn } from "@multica/ui/lib/utils";
import type { Agent } from "@multica/core/types";
import { AgentCard } from "./agent-card";

/**
 * Hierarchical org chart: pure CSS tree using flex columns + border
 * pseudo-lines to draw connectors between a parent card and its
 * direct reports. No SVG, no measurement, no external dependency.
 *
 * Connector strategy:
 *   - Each child wrapper has an absolutely positioned 1px top border.
 *     The border's left/right extent is clipped per position:
 *       • first child:  starts at its own center, extends to the right
 *       • last child:   extends from the left, ends at its own center
 *       • middle child: spans full width
 *   - Each child also gets a short vertical stub dropping from the
 *     top border down to the top of its card.
 *   - The parent card has a single vertical stub from its bottom down
 *     to the height where the horizontal bar lives.
 *
 * This produces a clean pixel-perfect tree without any runtime layout
 * math — the CSS does all the work.
 */
export function OrgChart({ agents }: { agents: Agent[] }) {
  const { roots, childrenOf } = useMemo(() => {
    const byId = new Map(agents.map((a) => [a.id, a]));
    const children = new Map<string, Agent[]>();
    const rootList: Agent[] = [];

    for (const agent of agents) {
      if (agent.reports_to && byId.has(agent.reports_to)) {
        const list = children.get(agent.reports_to) ?? [];
        list.push(agent);
        children.set(agent.reports_to, list);
      } else {
        rootList.push(agent);
      }
    }

    // Stable display: alphabetical within each level.
    rootList.sort((a, b) => a.name.localeCompare(b.name));
    for (const list of children.values()) {
      list.sort((a, b) => a.name.localeCompare(b.name));
    }

    return { roots: rootList, childrenOf: children };
  }, [agents]);

  return (
    <div className="flex items-start justify-center gap-12">
      {roots.map((root) => (
        <TreeNode
          key={root.id}
          agent={root}
          childrenOf={childrenOf}
          visited={new Set()}
        />
      ))}
    </div>
  );
}

function TreeNode({
  agent,
  childrenOf,
  visited,
}: {
  agent: Agent;
  childrenOf: Map<string, Agent[]>;
  visited: Set<string>;
}) {
  // Cycle guard (defensive — server enforces this, but we shouldn't
  // infinite-loop the UI if data is ever corrupt).
  if (visited.has(agent.id)) return null;
  const nextVisited = new Set(visited);
  nextVisited.add(agent.id);

  const children = childrenOf.get(agent.id) ?? [];

  return (
    <div className="flex flex-col items-center">
      <AgentCard agent={agent} />

      {children.length > 0 && (
        <>
          {/* Vertical stub dropping from parent card to the horizontal bar */}
          <div className="h-6 w-px bg-border" />

          {/* Row of children with connector lines above each one */}
          <div className="flex items-start gap-6">
            {children.map((child, i) => {
              const isFirst = i === 0;
              const isLast = i === children.length - 1;
              const isOnly = children.length === 1;

              return (
                <div key={child.id} className="relative flex flex-col items-center pt-6">
                  {/* Horizontal connector bar — skipped when there's only one
                      child (the parent's vertical stub already reaches it). */}
                  {!isOnly && (
                    <div
                      className={cn(
                        "absolute top-0 h-px bg-border",
                        isFirst ? "left-1/2 right-0" : isLast ? "left-0 right-1/2" : "left-0 right-0",
                      )}
                      aria-hidden
                    />
                  )}

                  {/* Vertical drop from the horizontal bar to the child card.
                      For only-child we also draw a short stub so the line
                      visibly connects. */}
                  <div className="absolute top-0 h-6 w-px bg-border" aria-hidden />

                  <TreeNode
                    agent={child}
                    childrenOf={childrenOf}
                    visited={nextVisited}
                  />
                </div>
              );
            })}
          </div>
        </>
      )}
    </div>
  );
}
