package execenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InjectRuntimeConfig writes the meta skill content into the runtime-specific
// config file so the agent discovers its environment through its native mechanism.
//
// For Claude:   writes {workDir}/CLAUDE.md  (skills discovered natively from .claude/skills/)
// For Codex:    writes {workDir}/AGENTS.md  (skills discovered natively via CODEX_HOME)
// For OpenCode: writes {workDir}/AGENTS.md  (skills discovered natively from .config/opencode/skills/)
// For OpenClaw: writes {workDir}/AGENTS.md  (skills discovered natively from .openclaw/skills/)
func InjectRuntimeConfig(workDir, provider string, ctx TaskContextForEnv) error {
	content := buildMetaSkillContent(provider, ctx)

	switch provider {
	case "claude":
		return os.WriteFile(filepath.Join(workDir, "CLAUDE.md"), []byte(content), 0o644)
	case "codex", "opencode", "openclaw":
		return os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte(content), 0o644)
	default:
		// Unknown provider — skip config injection, prompt-only mode.
		return nil
	}
}

// InjectRuntimeConfigDirect writes the meta skill content into
// {workDir}/.agent_context/MULTICA_RUNTIME.md instead of the root CLAUDE.md
// or AGENTS.md file. Used in direct mode so the daemon never overwrites
// the user's existing agent-config files in a real project directory.
//
// The path returned should be referenced from the task prompt so the agent
// reads it explicitly at startup.
func InjectRuntimeConfigDirect(workDir, provider string, ctx TaskContextForEnv) (string, error) {
	contextDir := filepath.Join(workDir, ".agent_context")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return "", fmt.Errorf("create .agent_context dir: %w", err)
	}
	path := filepath.Join(contextDir, "MULTICA_RUNTIME.md")
	content := buildMetaSkillContent(provider, ctx)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write MULTICA_RUNTIME.md: %w", err)
	}
	return path, nil
}

// buildMetaSkillContent generates the meta skill markdown that teaches the agent
// about the Multica runtime environment and available CLI tools.
func buildMetaSkillContent(provider string, ctx TaskContextForEnv) string {
	var b strings.Builder

	b.WriteString("# Multica Agent Runtime\n\n")
	b.WriteString("You are a coding agent in the Multica platform. Use the `multica` CLI to interact with the platform.\n\n")

	// Inject agent identity instructions before workflow commands.
	if ctx.AgentInstructions != "" {
		b.WriteString("## Agent Identity\n\n")
		b.WriteString(ctx.AgentInstructions)
		b.WriteString("\n\n")
	}

	// Inject the agent's team — supervisor and direct reports — so it
	// knows who to @mention when delegating work up or down the chain.
	// This is what turns the `reports_to` field from metadata into a
	// live delegation mechanism: the agent has concrete IDs ready to
	// paste into comments, and the mention system already dispatches
	// tasks to the mentioned agent automatically.
	if ctx.Supervisor != nil || len(ctx.Subordinates) > 0 {
		b.WriteString("## Your Team\n\n")
		b.WriteString("You are part of an agent hierarchy with a supervisor and/or direct reports. **You are expected to delegate work that fits a teammate's role better than yours** — doing everything yourself when a subordinate could handle it is a bug, not a feature. Use @mentions in issue comments: mentions automatically spawn a task for the mentioned agent, who reads the issue, executes, and replies on the same thread.\n\n")

		if ctx.Supervisor != nil {
			s := ctx.Supervisor
			b.WriteString("### You report to\n\n")
			fmt.Fprintf(&b, "- **%s** — `[@%s](mention://agent/%s)`", s.Name, s.Name, s.ID)
			if s.Description != "" {
				fmt.Fprintf(&b, " — %s", s.Description)
			}
			b.WriteString("\n\n")
			b.WriteString("**Escalate to your supervisor when:**\n")
			b.WriteString("- The issue requires a strategic decision you don't have authority to make\n")
			b.WriteString("- Work you delegated downward is complete and needs final approval\n")
			b.WriteString("- You discover blocking problems (missing access, conflicting priorities, scope creep)\n")
			b.WriteString("- You finished what was asked and want confirmation before closing\n\n")
		}

		if len(ctx.Subordinates) > 0 {
			b.WriteString("### Your direct reports (delegate downward)\n\n")
			for _, sub := range ctx.Subordinates {
				fmt.Fprintf(&b, "- **%s** — `[@%s](mention://agent/%s)`", sub.Name, sub.Name, sub.ID)
				if sub.Description != "" {
					fmt.Fprintf(&b, " — %s", sub.Description)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
			b.WriteString("**Delegation checklist — run this BEFORE doing the work yourself:**\n")
			b.WriteString("1. Read the issue carefully\n")
			b.WriteString("2. Ask: \"does any of my direct reports' specialties fit this work better than my role?\"\n")
			b.WriteString("3. If yes → delegate. If no → do it yourself, then report to your supervisor if relevant\n")
			b.WriteString("4. When delegating, be specific in the comment — include context, acceptance criteria, files/areas involved\n\n")
			b.WriteString("**How to delegate (exact command):**\n\n")
			first := ctx.Subordinates[0]
			fmt.Fprintf(&b, "```\nmultica issue comment add <issue-id> --content \"[@%s](mention://agent/%s) <clear task with context>\"\n```\n\n", first.Name, first.ID)
			b.WriteString("The subordinate picks up the mention automatically, executes, and replies on the same issue thread. You can follow up with more mentions, review their output, and escalate to your supervisor when done.\n\n")
			b.WriteString("**Example flow:** Issue \"Add dark mode toggle to InAudit\" assigned to you →\n")
			b.WriteString("1. You recognize this is UI work\n")
			fmt.Fprintf(&b, "2. You post: `[@%s](mention://agent/%s) Implement a dark mode toggle in the InAudit header. Acceptance: persists per user, respects system default on first load, accessible via keyboard shortcut.`\n", first.Name, first.ID)
			b.WriteString("3. The subordinate takes over, makes the changes, and replies\n")
			b.WriteString("4. You review their reply, ask for adjustments if needed, then report completion to your supervisor (if you have one)\n\n")
		}
	}

	b.WriteString("## Available Commands\n\n")
	b.WriteString("**Always use `--output json` for all read commands** to get structured data with full IDs.\n\n")
	b.WriteString("### Read\n")
	b.WriteString("- `multica issue get <id> --output json` — Get full issue details (title, description, status, priority, assignee)\n")
	b.WriteString("- `multica issue list [--status X] [--priority X] [--assignee X] --output json` — List issues in workspace\n")
	b.WriteString("- `multica issue comment list <issue-id> [--limit N] [--offset N] [--since <RFC3339>] --output json` — List comments on an issue (supports pagination; includes id, parent_id for threading)\n")
	b.WriteString("- `multica workspace get --output json` — Get workspace details and context\n")
	b.WriteString("- `multica workspace members [workspace-id] --output json` — List workspace members (user IDs, names, roles)\n")
	b.WriteString("- `multica agent list --output json` — List agents in workspace\n")
	b.WriteString("- `multica repo checkout <url>` — Check out a repository into the working directory (creates a git worktree with a dedicated branch)\n")
	b.WriteString("- `multica issue runs <issue-id> --output json` — List all execution runs for an issue (status, timestamps, errors)\n")
	b.WriteString("- `multica issue run-messages <task-id> [--since <seq>] --output json` — List messages for a specific execution run (supports incremental fetch)\n")
	b.WriteString("- `multica attachment download <id> [-o <dir>]` — Download an attachment file locally by ID\n\n")

	b.WriteString("### Write\n")
	b.WriteString("- `multica issue create --title \"...\" [--description \"...\"] [--priority X] [--assignee X] [--parent <issue-id>] [--status X]` — Create a new issue\n")
	b.WriteString("- `multica issue assign <id> --to <name>` — Assign an issue to a member or agent by name (use --unassign to remove assignee)\n")
	b.WriteString("- `multica issue comment add <issue-id> --content \"...\" [--parent <comment-id>]` — Post a comment (use --parent to reply to a specific comment)\n")
	b.WriteString("- `multica issue comment delete <comment-id>` — Delete a comment\n")
	b.WriteString("- `multica issue status <id> <status>` — Update issue status (todo, in_progress, in_review, done, blocked)\n")
	b.WriteString("- `multica issue update <id> [--title X] [--description X] [--priority X]` — Update issue fields\n\n")

	// Inject available repositories section.
	if len(ctx.Repos) > 0 {
		b.WriteString("## Repositories\n\n")
		b.WriteString("The following code repositories are available in this workspace.\n")
		b.WriteString("Use `multica repo checkout <url>` to check out a repository into your working directory.\n\n")
		b.WriteString("| URL | Description |\n")
		b.WriteString("|-----|-------------|\n")
		for _, repo := range ctx.Repos {
			desc := repo.Description
			if desc == "" {
				desc = "—"
			}
			fmt.Fprintf(&b, "| %s | %s |\n", repo.URL, desc)
		}
		b.WriteString("\nThe checkout command creates a git worktree with a dedicated branch. You can check out one or more repos as needed.\n\n")
	}

	b.WriteString("### Workflow\n\n")

	if ctx.ChatSessionID != "" {
		// Chat task: interactive assistant mode
		b.WriteString("**You are in chat mode.** A user is messaging you directly in a chat window.\n\n")
		b.WriteString("- Respond conversationally and helpfully to the user's message\n")
		b.WriteString("- You have full access to the `multica` CLI to look up issues, workspace info, members, agents, etc.\n")
		b.WriteString("- If asked about issues, use `multica issue list --output json` or `multica issue get <id> --output json`\n")
		b.WriteString("- If asked about the workspace, use `multica workspace get --output json`\n")
		b.WriteString("- If asked to perform actions (create issues, update status, etc.), use the appropriate CLI commands\n")
		b.WriteString("- If the task requires code changes, use `multica repo checkout <url>` to get the code first\n")
		b.WriteString("- Keep responses concise and direct\n\n")
	} else if ctx.TriggerCommentID != "" {
		// Comment-triggered: focus on reading and replying
		b.WriteString("**This task was triggered by a comment.** Your primary job is to respond.\n\n")
		fmt.Fprintf(&b, "1. Run `multica issue get %s --output json` to understand the issue context\n", ctx.IssueID)
		fmt.Fprintf(&b, "2. Run `multica issue comment list %s --output json` to read the conversation\n", ctx.IssueID)
		b.WriteString("   - If the output is very large or truncated, use pagination: `--limit 30` to get the latest 30 comments, or `--since <timestamp>` to fetch only recent ones\n")
		fmt.Fprintf(&b, "3. Find the triggering comment (ID: `%s`) and understand what is being asked\n", ctx.TriggerCommentID)
		fmt.Fprintf(&b, "4. Reply: `multica issue comment add %s --parent %s --content \"...\"`\n", ctx.IssueID, ctx.TriggerCommentID)
		b.WriteString("5. If the comment requests code changes or further work, do the work first, then reply with your results\n")
		b.WriteString("6. Do NOT change the issue status unless the comment explicitly asks for it\n\n")
	} else {
		// Assignment-triggered: defer to agent Skills for workflow specifics.
		b.WriteString("You are responsible for managing the issue status throughout your work.\n\n")
		fmt.Fprintf(&b, "1. Run `multica issue get %s --output json` to understand your task\n", ctx.IssueID)
		fmt.Fprintf(&b, "2. Run `multica issue status %s in_progress`\n", ctx.IssueID)
		b.WriteString("3. Read comments for additional context or human instructions\n")
		b.WriteString("4. Follow your Skills and Agent Identity to determine how to complete this task.\n")
		b.WriteString("   If no relevant skill applies, the default workflow is: understand the task → do the work → post a comment with results → update issue status.\n")
		fmt.Fprintf(&b, "5. When done, run `multica issue status %s in_review`\n", ctx.IssueID)
		fmt.Fprintf(&b, "6. If blocked, run `multica issue status %s blocked` and post a comment explaining why\n\n", ctx.IssueID)
	}

	if len(ctx.AgentSkills) > 0 {
		b.WriteString("## Skills\n\n")
		switch provider {
		case "claude":
			// Claude discovers skills natively from .claude/skills/ — just list names.
			b.WriteString("You have the following skills installed (discovered automatically):\n\n")
		case "codex", "opencode", "openclaw":
			// Codex, OpenCode, and OpenClaw discover skills natively from their respective paths — just list names.
			b.WriteString("You have the following skills installed (discovered automatically):\n\n")
		default:
			b.WriteString("Detailed skill instructions are in `.agent_context/skills/`. Each subdirectory contains a `SKILL.md`.\n\n")
		}
		for _, skill := range ctx.AgentSkills {
			fmt.Fprintf(&b, "- **%s**\n", skill.Name)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Mentions\n\n")
	b.WriteString("When referencing issues or people in comments, use the mention format so they render as interactive links:\n\n")
	b.WriteString("- **Issue**: `[MUL-123](mention://issue/<issue-id>)` — renders as a clickable link to the issue\n")
	b.WriteString("- **Member**: `[@Name](mention://member/<user-id>)` — renders as a styled mention and sends a notification\n")
	b.WriteString("- **Agent**: `[@Name](mention://agent/<agent-id>)` — renders as a styled mention\n\n")
	b.WriteString("Use `multica issue list --output json` to look up issue IDs, and `multica workspace members --output json` for member IDs.\n\n")

	b.WriteString("## Attachments\n\n")
	b.WriteString("Issues and comments may include file attachments (images, documents, etc.).\n")
	b.WriteString("Use the download command to fetch attachment files locally:\n\n")
	b.WriteString("```\nmultica attachment download <attachment-id>\n```\n\n")
	b.WriteString("This downloads the file to the current directory and prints the local path. Use `-o <dir>` to save elsewhere.\n")
	b.WriteString("After downloading, you can read the file directly (e.g. view an image, read a document).\n\n")

	b.WriteString("## Important: Always Use the `multica` CLI\n\n")
	b.WriteString("All interactions with Multica platform resources — including issues, comments, attachments, images, files, and any other platform data — **must** go through the `multica` CLI. ")
	b.WriteString("Do NOT use `curl`, `wget`, or any other HTTP client to access Multica URLs or APIs directly. ")
	b.WriteString("Multica resource URLs require authenticated access that only the `multica` CLI can provide.\n\n")
	b.WriteString("If you need to perform an operation that is not covered by any existing `multica` command, ")
	b.WriteString("do NOT attempt to work around it. Instead, post a comment mentioning the workspace owner to request the missing functionality.\n\n")

	b.WriteString("## Output\n\n")
	b.WriteString("Keep comments concise and natural — state the outcome, not the process.\n")
	b.WriteString("Good: \"Fixed the login redirect. PR: https://...\"\n")
	b.WriteString("Bad: \"1. Read the issue 2. Found the bug in auth.go 3. Created branch 4. ...\"\n")

	return b.String()
}
