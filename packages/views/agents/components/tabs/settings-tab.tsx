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
  runtimes,
  onSave,
}: {
  agent: Agent;
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
    trimmedWorkdirPath !== initialWorkdirPath;

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
