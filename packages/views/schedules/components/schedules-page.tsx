"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Clock,
  Plus,
  Play,
  Pause,
  Trash2,
  Calendar,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { agentListOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { ActorAvatar } from "../../common/actor-avatar";
import type {
  ScheduledTask,
  CreateScheduledTaskRequest,
} from "@multica/core/types";
import { ScheduleDialog } from "./schedule-dialog";

/**
 * /schedules — manages recurring cron jobs that spawn issues at fixed
 * times. Each schedule belongs to a single agent and fires a fresh
 * issue with the stored prompt as the description.
 */
export function SchedulesPage() {
  const isLoading = useAuthStore((s) => s.isLoading);
  const wsId = useWorkspaceId();
  const qc = useQueryClient();

  const { data: schedules = [], isLoading: listLoading } = useQuery({
    queryKey: ["schedules", wsId],
    queryFn: () => api.listSchedules(),
    enabled: !!wsId,
  });
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<ScheduledTask | null>(null);

  const agentById = useMemo(
    () => new Map(agents.map((a) => [a.id, a])),
    [agents],
  );

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: ["schedules", wsId] });

  const handleCreate = async (data: CreateScheduledTaskRequest) => {
    try {
      await api.createSchedule(data);
      invalidate();
      toast.success("Schedule created");
      setDialogOpen(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to create schedule");
    }
  };

  const handleUpdate = async (id: string, data: CreateScheduledTaskRequest) => {
    try {
      await api.updateSchedule(id, data);
      invalidate();
      toast.success("Schedule updated");
      setDialogOpen(false);
      setEditing(null);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to update schedule");
    }
  };

  const handleToggle = async (s: ScheduledTask) => {
    try {
      await api.updateSchedule(s.id, { enabled: !s.enabled });
      invalidate();
      toast.success(s.enabled ? "Schedule paused" : "Schedule resumed");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to toggle schedule");
    }
  };

  const handleDelete = async (s: ScheduledTask) => {
    if (!confirm(`Delete schedule "${s.title}"?`)) return;
    try {
      await api.deleteSchedule(s.id);
      invalidate();
      toast.success("Schedule deleted");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to delete schedule");
    }
  };

  if (isLoading || listLoading) {
    return (
      <div className="flex h-full flex-col p-8">
        <Skeleton className="h-8 w-48" />
        <div className="mt-6 space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-20 w-full rounded-lg" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="flex h-12 items-center justify-between border-b px-6">
        <div className="flex items-center gap-3">
          <h1 className="text-sm font-semibold">Schedules</h1>
          <span className="text-xs text-muted-foreground">
            {schedules.length} {schedules.length === 1 ? "schedule" : "schedules"}
          </span>
        </div>
        <Button
          size="sm"
          onClick={() => {
            setEditing(null);
            setDialogOpen(true);
          }}
        >
          <Plus className="h-3.5 w-3.5" />
          New schedule
        </Button>
      </header>

      <div className="flex-1 overflow-y-auto p-6">
        {schedules.length === 0 ? (
          <EmptyState onCreate={() => setDialogOpen(true)} />
        ) : (
          <div className="mx-auto max-w-3xl space-y-2">
            {schedules.map((s) => (
              <ScheduleRow
                key={s.id}
                schedule={s}
                agentName={agentById.get(s.agent_id)?.name ?? "(unknown agent)"}
                agentId={s.agent_id}
                onEdit={() => {
                  setEditing(s);
                  setDialogOpen(true);
                }}
                onToggle={() => handleToggle(s)}
                onDelete={() => handleDelete(s)}
              />
            ))}
          </div>
        )}
      </div>

      {dialogOpen && (
        <ScheduleDialog
          agents={agents.filter((a) => !a.archived_at)}
          editing={editing}
          onClose={() => {
            setDialogOpen(false);
            setEditing(null);
          }}
          onSubmit={(data) => {
            if (editing) {
              handleUpdate(editing.id, data);
            } else {
              handleCreate(data);
            }
          }}
        />
      )}
    </div>
  );
}

function ScheduleRow({
  schedule,
  agentName,
  agentId,
  onEdit,
  onToggle,
  onDelete,
}: {
  schedule: ScheduledTask;
  agentName: string;
  agentId: string;
  onEdit: () => void;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const nextRun = formatNextRun(schedule.next_run_at);
  const lastRun = schedule.last_run_at
    ? formatTimeAgo(schedule.last_run_at)
    : "never";

  return (
    <div
      className={`group rounded-lg border p-4 transition-colors ${
        schedule.enabled ? "bg-card hover:bg-muted/30" : "bg-muted/20 opacity-70"
      }`}
    >
      <div className="flex items-start gap-3">
        <button
          type="button"
          onClick={onEdit}
          className="min-w-0 flex-1 text-left"
        >
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-semibold">{schedule.title}</h3>
            {!schedule.enabled && (
              <span className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                Disabled
              </span>
            )}
          </div>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
            {schedule.prompt}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <ActorAvatar actorType="agent" actorId={agentId} size={16} />
              {agentName}
            </span>
            <span className="flex items-center gap-1">
              <Clock className="h-3 w-3" />
              <code className="rounded bg-muted px-1 py-0.5 font-mono text-[10px]">
                {schedule.cron_expr}
              </code>
            </span>
            <span className="flex items-center gap-1">
              <Calendar className="h-3 w-3" />
              Next: {nextRun}
            </span>
            <span className="text-muted-foreground/70">Last: {lastRun}</span>
          </div>
        </button>

        <div className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
          <button
            type="button"
            onClick={onToggle}
            title={schedule.enabled ? "Disable" : "Enable"}
            className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            {schedule.enabled ? (
              <Pause className="h-3.5 w-3.5" />
            ) : (
              <Play className="h-3.5 w-3.5" />
            )}
          </button>
          <button
            type="button"
            onClick={onDelete}
            title="Delete"
            className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center text-muted-foreground">
      <Clock className="h-12 w-12 text-muted-foreground/30" />
      <p className="mt-4 text-sm">No scheduled tasks yet.</p>
      <p className="mt-1 max-w-sm text-center text-xs text-muted-foreground/70">
        Create a schedule to make an agent run the same task on a recurring
        cadence — every Monday at 9am, every hour, whatever you need.
      </p>
      <Button size="sm" className="mt-4" onClick={onCreate}>
        <Plus className="h-3.5 w-3.5" />
        New schedule
      </Button>
    </div>
  );
}

function formatNextRun(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const diffMs = d.getTime() - now.getTime();
  if (diffMs < 0) return "now";
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return "< 1 min";
  if (diffMins < 60) return `in ${diffMins}m`;
  const diffHours = Math.floor(diffMs / 3600000);
  if (diffHours < 24) return `in ${diffHours}h`;
  const diffDays = Math.floor(diffMs / 86400000);
  if (diffDays < 7) return `in ${diffDays}d`;
  return d.toLocaleDateString();
}

function formatTimeAgo(dateStr: string): string {
  const d = new Date(dateStr);
  const diffMs = Date.now() - d.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMs / 3600000);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffMs / 86400000);
  return `${diffDays}d ago`;
}
