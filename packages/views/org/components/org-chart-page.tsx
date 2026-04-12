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
 * Interaction model (Figma/Miro style):
 *   - The chart is rendered inside an overflow-hidden canvas.
 *   - Position is held as a {x, y} state and applied via
 *     `transform: translate(Xpx, Ypx) scale(Z)`. This is the only
 *     approach that keeps pan working after a CSS scale, because
 *     `transform: scale` doesn't grow the element's box — a plain
 *     overflow-auto container would not know how to scroll it.
 *   - Mouse drag on empty canvas updates x/y directly.
 *   - Floating zoom toolbar in the upper-right corner.
 */
export function OrgChartPage() {
  const isLoading = useAuthStore((s) => s.isLoading);
  const wsId = useWorkspaceId();
  const { data: agents = [], isLoading: agentsLoading } = useQuery(agentListOptions(wsId));

  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const dragStart = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  const activeAgents = useMemo(
    () => agents.filter((a) => !a.archived_at),
    [agents],
  );

  const onMouseDown = (e: ReactMouseEvent<HTMLDivElement>) => {
    // Ignore presses that land on interactive elements — we still want
    // links and buttons to behave normally inside the canvas.
    if ((e.target as HTMLElement).closest("a, button, input, textarea")) return;
    dragStart.current = {
      x: e.clientX,
      y: e.clientY,
      panX: pan.x,
      panY: pan.y,
    };
    setIsDragging(true);
    e.preventDefault();
  };

  const onMouseMove = (e: ReactMouseEvent<HTMLDivElement>) => {
    const start = dragStart.current;
    if (!start) return;
    setPan({
      x: start.panX + (e.clientX - start.x),
      y: start.panY + (e.clientY - start.y),
    });
  };

  const endDrag = () => {
    dragStart.current = null;
    setIsDragging(false);
  };

  const resetView = () => {
    setZoom(1);
    setPan({ x: 0, y: 0 });
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
            onMouseDown={onMouseDown}
            onMouseMove={onMouseMove}
            onMouseUp={endDrag}
            onMouseLeave={endDrag}
            className={`relative flex-1 select-none overflow-hidden ${
              isDragging ? "cursor-grabbing" : "cursor-grab"
            }`}
          >
            <div
              className="absolute left-1/2 top-10"
              style={{
                transform: `translate(calc(-50% + ${pan.x}px), ${pan.y}px) scale(${zoom})`,
                transformOrigin: "top center",
                transition: isDragging ? "none" : "transform 150ms ease-out",
              }}
            >
              <OrgChart agents={activeAgents} />
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
            onFit={resetView}
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
        aria-label="Reset view"
        title={`${Math.round(zoom * 100)}% — click to reset view`}
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
