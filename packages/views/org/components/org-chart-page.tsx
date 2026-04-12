"use client";

import { useMemo, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
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
 * Full-page org chart view.
 *
 * Interaction:
 *   - Click-and-drag on the canvas pans the chart (cursor: grab/grabbing),
 *     implemented by translating a mouse drag into scrollLeft/scrollTop
 *     deltas on the scroll container. No external lib.
 *   - Floating zoom toolbar in the upper-right corner:
 *       +   zoom in  (×1.1, capped at 2.0)
 *       −   zoom out (÷1.1, capped at 0.5)
 *       Fit reset to 100%
 *     The toolbar sits top-right rather than bottom-right to avoid
 *     overlapping the "Ask Multica" chat launcher.
 */
export function OrgChartPage() {
  const isLoading = useAuthStore((s) => s.isLoading);
  const wsId = useWorkspaceId();
  const { data: agents = [], isLoading: agentsLoading } = useQuery(agentListOptions(wsId));

  const [zoom, setZoom] = useState(1);
  const scrollRef = useRef<HTMLDivElement>(null);
  const dragState = useRef<{
    startX: number;
    startY: number;
    scrollLeft: number;
    scrollTop: number;
  } | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  const activeAgents = useMemo(
    () => agents.filter((a) => !a.archived_at),
    [agents],
  );

  const onMouseDown = (e: ReactMouseEvent<HTMLDivElement>) => {
    // Let clicks on cards (links) work normally — only start pan when
    // the user presses on the canvas background itself.
    if ((e.target as HTMLElement).closest("a, button, input, textarea")) return;
    const el = scrollRef.current;
    if (!el) return;
    dragState.current = {
      startX: e.clientX,
      startY: e.clientY,
      scrollLeft: el.scrollLeft,
      scrollTop: el.scrollTop,
    };
    setIsDragging(true);
    // Avoid text selection while dragging.
    e.preventDefault();
  };

  const onMouseMove = (e: ReactMouseEvent<HTMLDivElement>) => {
    const st = dragState.current;
    const el = scrollRef.current;
    if (!st || !el) return;
    el.scrollLeft = st.scrollLeft - (e.clientX - st.startX);
    el.scrollTop = st.scrollTop - (e.clientY - st.startY);
  };

  const endDrag = () => {
    dragState.current = null;
    setIsDragging(false);
  };

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
          <div
            ref={scrollRef}
            onMouseDown={onMouseDown}
            onMouseMove={onMouseMove}
            onMouseUp={endDrag}
            onMouseLeave={endDrag}
            className={`flex-1 select-none overflow-auto ${
              isDragging ? "cursor-grabbing" : "cursor-grab"
            }`}
          >
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
    <div className="absolute right-6 top-16 flex flex-col gap-1">
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
