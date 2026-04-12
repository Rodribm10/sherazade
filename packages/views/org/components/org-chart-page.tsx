"use client";

import { useMemo, useState } from "react";
import { Bot, Plus, Minus } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { agentListOptions } from "@multica/core/workspace/queries";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { OrgChart } from "./org-chart";

const MIN_ZOOM = 0.5;
const MAX_ZOOM = 2.0;
const ZOOM_STEP = 1.1;

/**
 * Full-page org chart view. Loads all agents for the current workspace
 * and renders them as a hierarchical card tree driven by the
 * `reports_to` field. Archived agents are filtered out.
 *
 * Includes a floating zoom toolbar in the bottom-right corner:
 *   +   zoom in (×1.1, capped at 2.0)
 *   −   zoom out (÷1.1, capped at 0.5)
 *   Fit reset to 100%
 *
 * Zoom is applied via CSS transform on the chart wrapper, so the
 * surrounding scroll container can still scroll through oversized
 * content without measuring anything.
 */
export function OrgChartPage() {
  const isLoading = useAuthStore((s) => s.isLoading);
  const wsId = useWorkspaceId();
  const { data: agents = [], isLoading: agentsLoading } = useQuery(agentListOptions(wsId));

  const [zoom, setZoom] = useState(1);

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
    <div className="relative flex h-full flex-col overflow-hidden">
      <header className="flex h-12 items-center border-b px-6">
        <h1 className="text-sm font-semibold">Org Chart</h1>
        <span className="ml-3 text-xs text-muted-foreground">
          {activeAgents.length} {activeAgents.length === 1 ? "agent" : "agents"}
        </span>
      </header>

      {activeAgents.length === 0 ? (
        <EmptyState />
      ) : (
        <>
          <div className="flex-1 overflow-auto">
            <div className="min-w-max p-10">
              <div
                className="origin-top transition-transform duration-150 ease-out"
                style={{ transform: `scale(${zoom})` }}
              >
                <OrgChart agents={activeAgents} />
              </div>
            </div>
          </div>
          <ZoomToolbar
            zoom={zoom}
            onZoomIn={() =>
              setZoom((z) => Math.min(MAX_ZOOM, +(z * ZOOM_STEP).toFixed(3)))
            }
            onZoomOut={() =>
              setZoom((z) => Math.max(MIN_ZOOM, +(z / ZOOM_STEP).toFixed(3)))
            }
            onFit={() => setZoom(1)}
          />
        </>
      )}
    </div>
  );
}

function ZoomToolbar({
  zoom,
  onZoomIn,
  onZoomOut,
  onFit,
}: {
  zoom: number;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFit: () => void;
}) {
  return (
    <div className="absolute bottom-6 right-6 flex flex-col gap-1">
      <button
        type="button"
        onClick={onZoomIn}
        disabled={zoom >= MAX_ZOOM}
        aria-label="Zoom in"
        className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-background text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
      >
        <Plus className="h-4 w-4" />
      </button>
      <button
        type="button"
        onClick={onZoomOut}
        disabled={zoom <= MIN_ZOOM}
        aria-label="Zoom out"
        className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-background text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
      >
        <Minus className="h-4 w-4" />
      </button>
      <button
        type="button"
        onClick={onFit}
        aria-label="Reset zoom"
        title={`${Math.round(zoom * 100)}% → 100%`}
        className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-background text-xs font-medium text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground"
      >
        Fit
      </button>
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
