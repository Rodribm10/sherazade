"use client";

import { useState, useEffect } from "react";
import { Bot, ChevronDown, Clock } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@multica/ui/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@multica/ui/components/ui/dropdown-menu";
import { ActorAvatar } from "../../common/actor-avatar";
import type {
  Agent,
  ScheduledTask,
  CreateScheduledTaskRequest,
} from "@multica/core/types";

/**
 * Preset cron expressions so users don't need to learn crontab.
 * "Custom" lets them type their own 5-field expression.
 */
const PRESETS: { label: string; value: string; hint?: string }[] = [
  { label: "Every hour", value: "0 * * * *" },
  { label: "Every day at 9:00", value: "0 9 * * *" },
  { label: "Every weekday at 9:00", value: "0 9 * * 1-5", hint: "Mon–Fri" },
  { label: "Every Monday at 9:00", value: "0 9 * * 1" },
  { label: "Every 15 minutes", value: "*/15 * * * *" },
  { label: "Every 5 minutes", value: "*/5 * * * *", hint: "fast testing" },
];

export function ScheduleDialog({
  agents,
  editing,
  onClose,
  onSubmit,
}: {
  agents: Agent[];
  editing: ScheduledTask | null;
  onClose: () => void;
  onSubmit: (data: CreateScheduledTaskRequest) => void;
}) {
  const [agentId, setAgentId] = useState(editing?.agent_id ?? agents[0]?.id ?? "");
  const [title, setTitle] = useState(editing?.title ?? "");
  const [prompt, setPrompt] = useState(editing?.prompt ?? "");
  const [cronExpr, setCronExpr] = useState(editing?.cron_expr ?? "0 9 * * 1");

  // When the list of agents loads after the dialog mounts, pick one.
  useEffect(() => {
    if (!agentId && agents.length > 0) {
      setAgentId(agents[0]!.id);
    }
  }, [agents, agentId]);

  const selectedAgent = agents.find((a) => a.id === agentId);
  const activePreset = PRESETS.find((p) => p.value === cronExpr);
  const isCustom = !activePreset;

  const handleSubmit = () => {
    if (!agentId) return;
    if (!title.trim() || !prompt.trim() || !cronExpr.trim()) return;
    onSubmit({
      agent_id: agentId,
      title: title.trim(),
      prompt: prompt.trim(),
      cron_expr: cronExpr.trim(),
    });
  };

  const valid =
    !!agentId && title.trim() !== "" && prompt.trim() !== "" && cronExpr.trim() !== "";

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-base">
            {editing ? "Edit schedule" : "New schedule"}
          </DialogTitle>
          <DialogDescription className="text-xs">
            Each fire creates a new issue with the prompt as description,
            assigned to the chosen agent.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <Label className="text-xs text-muted-foreground">Agent</Label>
            <DropdownMenu>
              <DropdownMenuTrigger className="mt-1 flex w-full items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-left text-sm transition-colors hover:bg-muted disabled:opacity-50" disabled={agents.length === 0}>
                {selectedAgent ? (
                  <>
                    <ActorAvatar actorType="agent" actorId={selectedAgent.id} size={20} />
                    <span className="flex-1 truncate font-medium">{selectedAgent.name}</span>
                  </>
                ) : (
                  <>
                    <Bot className="h-4 w-4 text-muted-foreground" />
                    <span className="flex-1 text-muted-foreground">No agents available</span>
                  </>
                )}
                <ChevronDown className="h-3 w-3 text-muted-foreground" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-[var(--anchor-width)]">
                {agents.map((agent) => (
                  <DropdownMenuItem key={agent.id} onClick={() => setAgentId(agent.id)}>
                    <ActorAvatar actorType="agent" actorId={agent.id} size={20} />
                    <span className="truncate">{agent.name}</span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          <div>
            <Label className="text-xs text-muted-foreground">Title</Label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Daily KPI report"
              className="mt-1"
            />
          </div>

          <div>
            <Label className="text-xs text-muted-foreground">
              Prompt (becomes the issue description)
            </Label>
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="Check failing tests in InAudit360 and post a summary here."
              rows={4}
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          <div>
            <Label className="text-xs text-muted-foreground">Schedule</Label>
            <div className="mt-1.5 grid grid-cols-2 gap-1.5">
              {PRESETS.map((preset) => (
                <button
                  key={preset.value}
                  type="button"
                  onClick={() => setCronExpr(preset.value)}
                  className={`flex flex-col items-start gap-0.5 rounded-md border px-2.5 py-1.5 text-left text-xs transition-colors ${
                    cronExpr === preset.value
                      ? "border-primary bg-primary/5"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  <span className="font-medium">{preset.label}</span>
                  {preset.hint && (
                    <span className="text-[10px] text-muted-foreground">{preset.hint}</span>
                  )}
                </button>
              ))}
            </div>
            <div className="mt-2 flex items-center gap-2">
              <Clock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <Input
                value={cronExpr}
                onChange={(e) => setCronExpr(e.target.value)}
                placeholder="0 9 * * 1"
                className="font-mono text-xs"
              />
              {isCustom && (
                <span className="shrink-0 text-[10px] text-muted-foreground">custom</span>
              )}
            </div>
            <p className="mt-1 text-[11px] text-muted-foreground">
              Standard 5-field cron: minute hour day-of-month month day-of-week.
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!valid}>
            {editing ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
