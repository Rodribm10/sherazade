"use client";

import { useState, useRef } from "react";
import {
  Cloud,
  Monitor,
  Loader2,
  Save,
  Globe,
  Lock,
  Camera,
  ChevronDown,
  FolderLock,
  FolderOpen,
  AlertTriangle,
  Network,
  X,
  Wallet,
} from "lucide-react";
import type {
  Agent,
  AgentRuntimeConfig,
  AgentVisibility,
  RuntimeDevice,
} from "@multica/core/types";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@multica/ui/components/ui/popover";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { ActorAvatar } from "../../../common/actor-avatar";

export function SettingsTab({
  agent,
  agents,
  runtimes,
  onSave,
}: {
  agent: Agent;
  agents: Agent[];
  runtimes: RuntimeDevice[];
  onSave: (updates: Partial<Agent>) => Promise<void>;
}) {
  const [name, setName] = useState(agent.name);
  const [description, setDescription] = useState(agent.description ?? "");
  const [visibility, setVisibility] = useState<AgentVisibility>(agent.visibility);
  const [maxTasks, setMaxTasks] = useState(agent.max_concurrent_tasks);
  const [selectedRuntimeId, setSelectedRuntimeId] = useState(agent.runtime_id);
  const [runtimeOpen, setRuntimeOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  // Workdir mode: "isolated" (default) or "direct" (agent edits real host files)
  const initialWorkdirMode: "isolated" | "direct" =
    agent.runtime_config?.workdir_mode === "direct" ? "direct" : "isolated";
  const initialWorkdirPath = agent.runtime_config?.workdir_path ?? "";
  const [workdirMode, setWorkdirMode] = useState<"isolated" | "direct">(initialWorkdirMode);
  const [workdirPath, setWorkdirPath] = useState(initialWorkdirPath);

  // Reports-to: org chart hierarchy
  const [reportsTo, setReportsTo] = useState<string | null>(agent.reports_to);
  const [reportsToOpen, setReportsToOpen] = useState(false);

  // Budget: stored as cents on the server, exposed in dollars in the UI.
  // Empty string means "unlimited".
  const initialBudgetDollars =
    agent.budget_monthly_cents !== null
      ? (agent.budget_monthly_cents / 100).toFixed(2)
      : "";
  const [budgetDollars, setBudgetDollars] = useState(initialBudgetDollars);
  const spentDollars = agent.spent_monthly_cents / 100;
  const budgetCentsParsed = (() => {
    const t = budgetDollars.trim();
    if (t === "") return null;
    const parsed = Number(t);
    if (!Number.isFinite(parsed) || parsed < 0) return NaN;
    return Math.round(parsed * 100);
  })();
  const budgetPct =
    agent.budget_monthly_cents && agent.budget_monthly_cents > 0
      ? Math.min(100, (agent.spent_monthly_cents / agent.budget_monthly_cents) * 100)
      : 0;

  // Compute descendants of THIS agent so we prevent selecting one as supervisor
  // (would create a cycle). Done client-side for instant feedback; the server
  // also enforces this on save.
  const descendantIds = (() => {
    const result = new Set<string>();
    const stack = [agent.id];
    while (stack.length > 0) {
      const currentId = stack.pop()!;
      for (const a of agents) {
        if (a.reports_to === currentId && !result.has(a.id)) {
          result.add(a.id);
          stack.push(a.id);
        }
      }
    }
    return result;
  })();

  // Valid supervisor candidates: any non-archived agent in the workspace
  // that isn't this agent and isn't one of its descendants.
  const supervisorCandidates = agents.filter(
    (a) => a.id !== agent.id && !descendantIds.has(a.id) && !a.archived_at,
  );
  const supervisorAgent = agents.find((a) => a.id === reportsTo) ?? null;
  const { upload, uploading } = useFileUpload(api);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const selectedRuntime = runtimes.find((d) => d.id === selectedRuntimeId) ?? null;

  const handleAvatarUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = "";
    try {
      const result = await upload(file);
      if (!result) return;
      await onSave({ avatar_url: result.link });
      toast.success("Avatar updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to upload avatar");
    }
  };

  const trimmedWorkdirPath = workdirPath.trim();
  const dirty =
    name !== agent.name ||
    description !== (agent.description ?? "") ||
    visibility !== agent.visibility ||
    maxTasks !== agent.max_concurrent_tasks ||
    selectedRuntimeId !== agent.runtime_id ||
    workdirMode !== initialWorkdirMode ||
    trimmedWorkdirPath !== initialWorkdirPath ||
    reportsTo !== agent.reports_to ||
    budgetDollars !== initialBudgetDollars;

  const handleSave = async () => {
    if (!name.trim()) {
      toast.error("Name is required");
      return;
    }
    if (workdirMode === "direct") {
      if (!trimmedWorkdirPath) {
        toast.error("Workdir path is required when using direct mode");
        return;
      }
      if (!trimmedWorkdirPath.startsWith("/")) {
        toast.error("Workdir path must be an absolute path (start with /)");
        return;
      }
    }
    if (Number.isNaN(budgetCentsParsed)) {
      toast.error("Budget must be a non-negative number in dollars");
      return;
    }
    setSaving(true);
    try {
      // Preserve any unknown keys already stored in runtime_config so this
      // form doesn't clobber future settings it doesn't know about.
      const nextRuntimeConfig: AgentRuntimeConfig = {
        ...(agent.runtime_config ?? {}),
      };
      if (workdirMode === "direct") {
        nextRuntimeConfig.workdir_mode = "direct";
        nextRuntimeConfig.workdir_path = trimmedWorkdirPath;
      } else {
        // Isolated is the default — drop the keys entirely.
        delete nextRuntimeConfig.workdir_mode;
        delete nextRuntimeConfig.workdir_path;
      }
      await onSave({
        name: name.trim(),
        description,
        visibility,
        max_concurrent_tasks: maxTasks,
        runtime_id: selectedRuntimeId,
        runtime_config: nextRuntimeConfig,
        // Only send reports_to when it actually changed, so we don't
        // include it on saves that don't touch the hierarchy.
        ...(reportsTo !== agent.reports_to && { reports_to: reportsTo }),
        // Same thing for budget: only include when changed.
        ...(budgetDollars !== initialBudgetDollars && {
          budget_monthly_cents: budgetCentsParsed,
        }),
      });
      toast.success("Settings saved");
    } catch {
      toast.error("Failed to save settings");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="max-w-lg space-y-6">
      <div>
        <Label className="text-xs text-muted-foreground">Avatar</Label>
        <div className="mt-1.5 flex items-center gap-4">
          <button
            type="button"
            className="group relative h-16 w-16 shrink-0 rounded-full bg-muted overflow-hidden focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
          >
            <ActorAvatar actorType="agent" actorId={agent.id} size={64} className="rounded-none" />
            <div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
              {uploading ? (
                <Loader2 className="h-5 w-5 animate-spin text-white" />
              ) : (
                <Camera className="h-5 w-5 text-white" />
              )}
            </div>
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={handleAvatarUpload}
          />
          <div className="text-xs text-muted-foreground">
            Click to upload avatar
          </div>
        </div>
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Name</Label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mt-1"
        />
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Description</Label>
        <Input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="What does this agent do?"
          className="mt-1"
        />
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Visibility</Label>
        <div className="mt-1.5 flex gap-2">
          <button
            type="button"
            onClick={() => setVisibility("workspace")}
            className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
              visibility === "workspace"
                ? "border-primary bg-primary/5"
                : "border-border hover:bg-muted"
            }`}
          >
            <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
            <div className="text-left">
              <div className="font-medium">Workspace</div>
              <div className="text-xs text-muted-foreground">All members can assign</div>
            </div>
          </button>
          <button
            type="button"
            onClick={() => setVisibility("private")}
            className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
              visibility === "private"
                ? "border-primary bg-primary/5"
                : "border-border hover:bg-muted"
            }`}
          >
            <Lock className="h-4 w-4 shrink-0 text-muted-foreground" />
            <div className="text-left">
              <div className="font-medium">Private</div>
              <div className="text-xs text-muted-foreground">Only you can assign</div>
            </div>
          </button>
        </div>
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Max Concurrent Tasks</Label>
        <Input
          type="number"
          min={1}
          max={50}
          value={maxTasks}
          onChange={(e) => setMaxTasks(Number(e.target.value))}
          className="mt-1 w-24"
        />
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Monthly Budget (USD)</Label>
        <div className="mt-1 flex items-center gap-2">
          <div className="flex items-center gap-1.5 rounded-md border border-border bg-background px-2.5">
            <Wallet className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">$</span>
            <Input
              type="number"
              min={0}
              step="0.01"
              value={budgetDollars}
              onChange={(e) => setBudgetDollars(e.target.value)}
              placeholder="Unlimited"
              className="h-8 w-28 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
            />
          </div>
          {budgetDollars.trim() !== "" && (
            <button
              type="button"
              onClick={() => setBudgetDollars("")}
              className="text-xs text-muted-foreground hover:text-foreground underline underline-offset-2"
            >
              Clear
            </button>
          )}
        </div>
        <div className="mt-2 text-xs text-muted-foreground">
          Spent this month:{" "}
          <span className="font-medium text-foreground">
            ${spentDollars.toFixed(2)}
          </span>
          {agent.budget_monthly_cents !== null && (
            <>
              {" / "}
              <span className="font-medium text-foreground">
                ${(agent.budget_monthly_cents / 100).toFixed(2)}
              </span>
            </>
          )}
          {agent.budget_period_start && (
            <span className="ml-2 text-muted-foreground/70">
              (period {agent.budget_period_start})
            </span>
          )}
        </div>
        {agent.budget_monthly_cents !== null && agent.budget_monthly_cents > 0 && (
          <div className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-muted">
            <div
              className={`h-full transition-all ${
                budgetPct >= 100
                  ? "bg-destructive"
                  : budgetPct >= 80
                    ? "bg-warning"
                    : "bg-primary"
              }`}
              style={{ width: `${budgetPct}%` }}
            />
          </div>
        )}
        {agent.budget_monthly_cents !== null &&
          agent.spent_monthly_cents >= agent.budget_monthly_cents && (
            <div className="mt-2 flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 p-2 text-xs">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-destructive" />
              <span>
                Budget exceeded — new tasks for this agent are paused until the
                period rolls over on the 1st or the budget is raised.
              </span>
            </div>
          )}
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Runtime</Label>
        <Popover open={runtimeOpen} onOpenChange={setRuntimeOpen}>
          <PopoverTrigger
            disabled={runtimes.length === 0}
            className="flex w-full items-center gap-3 rounded-lg border border-border bg-background px-3 py-2.5 mt-1.5 text-left text-sm transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-50"
          >
            {selectedRuntime?.runtime_mode === "cloud" ? (
              <Cloud className="h-4 w-4 shrink-0 text-muted-foreground" />
            ) : (
              <Monitor className="h-4 w-4 shrink-0 text-muted-foreground" />
            )}
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate font-medium">
                  {selectedRuntime?.name ?? "No runtime available"}
                </span>
                {selectedRuntime?.runtime_mode === "cloud" && (
                  <span className="shrink-0 rounded bg-info/10 px-1.5 py-0.5 text-xs font-medium text-info">
                    Cloud
                  </span>
                )}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                {selectedRuntime?.device_info ?? "Select a runtime"}
              </div>
            </div>
            <ChevronDown className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${runtimeOpen ? "rotate-180" : ""}`} />
          </PopoverTrigger>
          <PopoverContent align="start" className="w-[var(--anchor-width)] p-1 max-h-60 overflow-y-auto">
            {runtimes.map((device) => (
              <button
                key={device.id}
                onClick={() => {
                  setSelectedRuntimeId(device.id);
                  setRuntimeOpen(false);
                }}
                className={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm transition-colors ${
                  device.id === selectedRuntimeId ? "bg-accent" : "hover:bg-accent/50"
                }`}
              >
                {device.runtime_mode === "cloud" ? (
                  <Cloud className="h-4 w-4 shrink-0 text-muted-foreground" />
                ) : (
                  <Monitor className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{device.name}</span>
                    {device.runtime_mode === "cloud" && (
                      <span className="shrink-0 rounded bg-info/10 px-1.5 py-0.5 text-xs font-medium text-info">
                        Cloud
                      </span>
                    )}
                  </div>
                  <div className="truncate text-xs text-muted-foreground">{device.device_info}</div>
                </div>
                <span
                  className={`h-2 w-2 shrink-0 rounded-full ${
                    device.status === "online" ? "bg-success" : "bg-muted-foreground/40"
                  }`}
                />
              </button>
            ))}
          </PopoverContent>
        </Popover>
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Reports To</Label>
        <Popover open={reportsToOpen} onOpenChange={setReportsToOpen}>
          <PopoverTrigger className="mt-1.5 flex w-full items-center gap-3 rounded-lg border border-border bg-background px-3 py-2.5 text-left text-sm transition-colors hover:bg-muted">
            <Network className="h-4 w-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium">
                {supervisorAgent?.name ?? "No supervisor"}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                {supervisorAgent
                  ? "Supervisor in the agent hierarchy"
                  : "Top-level agent (no one above)"}
              </div>
            </div>
            {supervisorAgent && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setReportsTo(null);
                }}
                className="shrink-0 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                aria-label="Clear supervisor"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
            <ChevronDown
              className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${
                reportsToOpen ? "rotate-180" : ""
              }`}
            />
          </PopoverTrigger>
          <PopoverContent
            align="start"
            className="w-[var(--anchor-width)] p-1 max-h-72 overflow-y-auto"
          >
            {supervisorCandidates.length === 0 ? (
              <div className="px-3 py-6 text-center text-xs text-muted-foreground">
                No other agents available to pick as supervisor.
              </div>
            ) : (
              supervisorCandidates.map((candidate) => (
                <button
                  key={candidate.id}
                  onClick={() => {
                    setReportsTo(candidate.id);
                    setReportsToOpen(false);
                  }}
                  className={`flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm transition-colors ${
                    candidate.id === reportsTo ? "bg-accent" : "hover:bg-accent/50"
                  }`}
                >
                  <ActorAvatar
                    actorType="agent"
                    actorId={candidate.id}
                    size={28}
                    className="shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{candidate.name}</div>
                    {candidate.description && (
                      <div className="truncate text-xs text-muted-foreground">
                        {candidate.description}
                      </div>
                    )}
                  </div>
                </button>
              ))
            )}
          </PopoverContent>
        </Popover>
      </div>

      <div>
        <Label className="text-xs text-muted-foreground">Working Directory</Label>
        <div className="mt-1.5 flex gap-2">
          <button
            type="button"
            onClick={() => setWorkdirMode("isolated")}
            className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
              workdirMode === "isolated"
                ? "border-primary bg-primary/5"
                : "border-border hover:bg-muted"
            }`}
          >
            <FolderLock className="h-4 w-4 shrink-0 text-muted-foreground" />
            <div className="text-left">
              <div className="font-medium">Isolated</div>
              <div className="text-xs text-muted-foreground">
                Sandbox per task (default)
              </div>
            </div>
          </button>
          <button
            type="button"
            onClick={() => setWorkdirMode("direct")}
            className={`flex flex-1 items-center gap-2 rounded-lg border px-3 py-2.5 text-sm transition-colors ${
              workdirMode === "direct"
                ? "border-primary bg-primary/5"
                : "border-border hover:bg-muted"
            }`}
          >
            <FolderOpen className="h-4 w-4 shrink-0 text-muted-foreground" />
            <div className="text-left">
              <div className="font-medium">Direct</div>
              <div className="text-xs text-muted-foreground">
                Edit real host files
              </div>
            </div>
          </button>
        </div>
        {workdirMode === "direct" && (
          <div className="mt-3 space-y-2">
            <Input
              value={workdirPath}
              onChange={(e) => setWorkdirPath(e.target.value)}
              placeholder="/Users/you/Dev/your-project"
              className="font-mono text-xs"
            />
            <div className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 p-2.5 text-xs">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-warning" />
              <div>
                <div className="font-medium text-warning">Direct mode is destructive.</div>
                <div className="mt-0.5 text-muted-foreground">
                  The agent will edit files at this path directly on the daemon host.
                  Your existing CLAUDE.md and .claude/ are left untouched; only a hidden
                  .agent_context/ folder is created. Use a dedicated project path and
                  make sure the agent runtime runs on the same machine.
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      <Button onClick={handleSave} disabled={!dirty || saving} size="sm">
        {saving ? <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" /> : <Save className="h-3.5 w-3.5 mr-1.5" />}
        Save Changes
      </Button>
    </div>
  );
}
