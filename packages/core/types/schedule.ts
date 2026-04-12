/**
 * A scheduled_task row: a recurring cron-style job that creates an
 * issue assigned to the given agent every time it fires.
 *
 * cron_expr is a standard 5-field crontab expression (minute, hour,
 * day-of-month, month, day-of-week). See server-side robfig/cron v3
 * for the exact grammar.
 */
export interface ScheduledTask {
  id: string;
  workspace_id: string;
  agent_id: string;
  created_by: string;
  title: string;
  prompt: string;
  cron_expr: string;
  enabled: boolean;
  last_run_at: string | null;
  next_run_at: string;
  created_at: string;
  updated_at: string;
}

export interface CreateScheduledTaskRequest {
  agent_id: string;
  title: string;
  prompt: string;
  cron_expr: string;
}

export interface UpdateScheduledTaskRequest {
  agent_id?: string;
  title?: string;
  prompt?: string;
  cron_expr?: string;
  enabled?: boolean;
}
