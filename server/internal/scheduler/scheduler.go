// Package scheduler runs the server-side cron-like loop that fires
// scheduled_task rows when they come due. It creates a new issue per
// fire (assigned to the schedule's agent) and advances next_run_at by
// re-evaluating the cron expression.
//
// Safety rules:
//   - The loop is single-threaded: one ticker, one worker. We rely on
//     ListDueScheduledTasks being idempotent: a schedule that we fire
//     now gets its next_run_at pushed to the future, so the next tick
//     will not pick it up again.
//   - Agents that are paused or archived are skipped silently; the
//     schedule stays due (next_run_at isn't advanced) so it fires
//     cleanly once the agent is unpaused.
//   - Cron parse errors disable the schedule after logging — we never
//     want the loop to crash on bad data.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TickInterval is how often the loop wakes up to look for due schedules.
// 60 seconds matches cron's minute-level granularity.
const TickInterval = 60 * time.Second

// cronParser accepts standard 5-field crontab expressions.
// Example: "0 9 * * 1" = every Monday at 09:00 local time.
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// Scheduler fires due scheduled_task rows, creating an issue per fire.
type Scheduler struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	taskService *service.TaskService
}

func New(pool *pgxpool.Pool, taskService *service.TaskService) *Scheduler {
	return &Scheduler{
		pool:        pool,
		queries:     db.New(pool),
		taskService: taskService,
	}
}

// Run blocks until ctx is cancelled. Meant to be launched in its own
// goroutine from main.
func (s *Scheduler) Run(ctx context.Context) {
	slog.Info("scheduler: starting", "tick_interval", TickInterval)
	t := time.NewTicker(TickInterval)
	defer t.Stop()

	// Run once immediately so schedules created just before startup
	// don't wait a full minute.
	s.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler: stopping")
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	due, err := s.queries.ListDueScheduledTasks(ctx)
	if err != nil {
		slog.Warn("scheduler: list due failed", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}
	slog.Info("scheduler: firing schedules", "count", len(due))

	for _, sched := range due {
		if err := s.fireSchedule(ctx, sched); err != nil {
			slog.Warn("scheduler: fire failed",
				"schedule_id", sched.ID.String(),
				"error", err,
			)
		}
	}
}

// fireSchedule creates an issue for the schedule (assigned to its agent)
// and advances next_run_at. Agent must exist, not be archived, and not
// be paused — otherwise we skip without advancing so the schedule fires
// once the agent is back.
func (s *Scheduler) fireSchedule(ctx context.Context, sched db.ScheduledTask) error {
	agent, err := s.queries.GetAgent(ctx, sched.AgentID)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}
	if agent.ArchivedAt.Valid {
		slog.Info("scheduler: skipping archived agent",
			"schedule_id", sched.ID.String(),
			"agent_id", agent.ID.String(),
		)
		return s.advanceNext(ctx, sched)
	}
	if agent.PausedAt.Valid {
		slog.Info("scheduler: skipping paused agent — keeping schedule due",
			"schedule_id", sched.ID.String(),
			"agent_id", agent.ID.String(),
		)
		return nil // don't advance: we'll retry on the next tick
	}

	// Create the issue inside a transaction (needed because the
	// workspace issue counter is a sequential increment). Once the
	// issue is committed we delegate to the same task service the
	// HTTP handler uses — that way the enqueue path stays identical
	// for manual assigns and scheduled fires.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	number, err := qtx.IncrementIssueCounter(ctx, sched.WorkspaceID)
	if err != nil {
		return fmt.Errorf("increment issue counter: %w", err)
	}

	issue, err := qtx.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:  sched.WorkspaceID,
		Title:        sched.Title,
		Description:  pgtype.Text{String: sched.Prompt, Valid: true},
		Status:       "todo",
		Priority:     "none",
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   sched.AgentID,
		CreatorType:  "user",
		CreatorID:    sched.CreatedBy,
		Number:       number,
	})
	if err != nil {
		return fmt.Errorf("create issue: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	// Enqueue the agent task via the same service the HTTP handler
	// uses. Any cancellations or dedup the service provides apply
	// here too.
	if _, err := s.taskService.EnqueueTaskForIssue(ctx, issue); err != nil {
		slog.Warn("scheduler: enqueue task failed (issue created, no task queued)",
			"schedule_id", sched.ID.String(),
			"issue_id", issue.ID.String(),
			"error", err,
		)
	}

	slog.Info("scheduler: fired schedule",
		"schedule_id", sched.ID.String(),
		"issue_id", issue.ID.String(),
		"agent_id", agent.ID.String(),
	)

	return s.advanceNext(ctx, sched)
}

// advanceNext parses the cron expression and writes the next run time
// back to the row. A parse failure disables the schedule — bad config
// shouldn't keep crashing the loop every minute.
func (s *Scheduler) advanceNext(ctx context.Context, sched db.ScheduledTask) error {
	next, err := ComputeNextRun(sched.CronExpr, time.Now())
	if err != nil {
		slog.Warn("scheduler: bad cron expr — disabling schedule",
			"schedule_id", sched.ID.String(),
			"cron_expr", sched.CronExpr,
			"error", err,
		)
		_, disableErr := s.queries.UpdateScheduledTask(ctx, db.UpdateScheduledTaskParams{
			ID:      sched.ID,
			Enabled: pgtype.Bool{Bool: false, Valid: true},
		})
		return disableErr
	}

	_, err = s.queries.MarkScheduledTaskRun(ctx, db.MarkScheduledTaskRunParams{
		ID:        sched.ID,
		NextRunAt: pgtype.Timestamptz{Time: next, Valid: true},
	})
	return err
}

// ComputeNextRun parses a 5-field cron expression and returns the next
// firing time strictly after `from`. Exposed so handlers can validate
// user input before saving and compute the initial next_run_at.
func ComputeNextRun(expr string, from time.Time) (time.Time, error) {
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}

