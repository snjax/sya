---
name: sya
description: >
  Use the sya git-native issue tracker during agent work: prime context, find and
  claim ready tasks, follow schema guards, update task body/logs, create discovered
  work, close/archive tasks, and use wisps for ephemeral notes.
allowed-tools: "Read,Bash(sya:*)"
version: "0.1.0"
author: "snjax"
license: "MIT"
---

# sya Agent Workflow

`sya` is a git-native issue tracker stored in `.sya/`. The tasks database is hidden from grep-style tools; `schema.yml`, `config.yml`, and `memory/` remain readable for agent context. Use sya when work needs to survive the current chat, coordinate with other agents, preserve task history, or participate in a schema-driven board. Use ephemeral in-chat todos only for short private execution checklists that do not need to persist.

## Start Of Session

Run:

```bash
sya prime
```

The Claude Code plugin also runs `sya prime` on `SessionStart` and `PreCompact`. Outside a sya project, `sya prime` must exit 0 silently, so the hook is safe globally.

Use the prime output to learn:

- current ready and in-progress tasks
- project schema and workflow digest
- memory notes relevant to tasks
- whether the repo already has active `.sya/` state

## Branch And Worktree Per Task

When the repository workflow allows it, use one branch or worktree per durable task:

```bash
git switch -c sya/<id>
```

Move the task through statuses on that branch as work progresses. Because `.sya/` is git-native, merging the branch merges the board movement, task log, and code change together.

## Find And Claim Work

Find ready tasks:

```bash
sya ready
sya ready --type feature --limit 5
sya ready --label cli --assignee codex
```

Inspect the board:

```bash
sya board
sya board --type task
```

Claim a task before doing durable work:

```bash
sya claim <id>
sya claim <id> --steal
```

Claim may fail with `already_claimed` exit 3. Do not overwrite another actor unless the user explicitly tells you to use `--steal`.

## Read The Task

```bash
sya show <id>
sya transitions <id>
sya list --type feature --status impl --limit 10
```

Use ID prefixes only when they resolve uniquely. Ambiguous prefix errors include `candidates`; choose the exact intended ID.

## Move Through The Workflow

Move one or more tasks:

```bash
sya move <id> <status>
sya move <id1> <id2> <status>
sya move <id> <status> --explain
```

Close terminal work:

```bash
sya close <id> --reason "implemented and verified"
sya close <id> --to scrapped --reason "superseded"
```

Reopen if necessary:

```bash
sya reopen <id>
sya reopen <id> --to todo
```

Guard errors tell you the fix. With `--json`, every error is:

```json
{"ok":false,"error":{"type":"transition_blocked","message":"transition blocked"}}
```

Read these fields literally:

- `violations`: each failed guard.
- `violations[].message`: why the move is blocked.
- `violations[].hint`: the next command or edit to make.
- `violations[].offending`: related tasks causing the block, usually dependencies or children.
- `alternatives`: transitions that currently pass.
- `allowed`: whitelist transitions when the requested transition is not allowed at all.

Default schema examples from `sya schema docs`:

- `task`: `todo -> in_progress -> done`; terminal: `done`, `scrapped`.
- `feature`: `draft -> spec -> impl -> review -> done`; `spec -> impl` requires `fields.spec_approved=true` and nonempty `Design`.
- `epic`: `active -> done` requires child tasks to be terminal.
- `depends_on` is a blocking DAG relation; targets must be terminal before dependent work can complete.

If a guard says a field is missing:

```bash
sya update <id> --field spec_approved=true
```

If a guard says a section is empty:

```bash
printf '%s\n' "Design text" | sya edit <id> --section Design --file -
```

If a dependency blocks:

```bash
sya show <offending-id>
sya move <offending-id> <next-status>
```

## Edit And Comment

Short operational notes go to `Log`:

```bash
sya comment <id> -m "Found failing integration case"
```

Structured task body edits use sections:

```bash
printf '%s\n' "New design notes" | sya edit <id> --section Design --file -
```

For task files edited directly, run:

```bash
sya doctor
```

Use `sya doctor --fix-merge` for Log-only merge conflicts. Use `sya doctor --reassign-id <id>` for duplicate IDs after merges.

## Discovered Work

When implementation reveals follow-up work, create a new task and link provenance:

```bash
sya create "Fix flaky restore test" -t bug --discovered-from <current-id>
sya create "Document schema migration" -t docs --depends-on <current-id>
```

Use `--rel relation=id` for nonstandard relations:

```bash
sya create "ADR: storage backend" -t decision --rel affects=<task-id>
```

## Analytics & Scaffolding

Use query expressions for precise searches:

```bash
sya query 'type=feature and not terminal and (age>7d or blocked)' --limit 10
sya query 'rel.depends_on and priority>=high'
sya query 'assignee=codex and working'
```

Use templates when a repeated task shape exists in `.sya/templates/`:

```bash
sya template list
sya template show feature-set
sya template apply feature-set -p name=Streaming --parent <epic-id>
sya template apply feature-set -p name=Streaming --dry-run
```

Use project analytics and planning outputs when deciding what to pick up or summarize:

```bash
sya stats
sya stale --days 14 --limit 10
sya duplicates --threshold 0.7 --limit 5
sya graph --all-relations --format mermaid
sya graph --epic <id>
sya roadmap
sya roadmap -o ROADMAP.md
```

If a transition reports an `attest` guard, read the question and provide a justified yes-answer only when it is true:

```bash
sya move <id> audit --attest human_review="yes: reviewed the diff and acceptance criteria with the human reviewer"
sya close <id> --attest release_ready="yes: tests passed and release notes were checked"
sya claim <id> --attest handoff_ok="yes: the previous owner explicitly handed this task off"
```

Do not use attestations as a shortcut around missing evidence. If the answer is not honestly yes, fix the task or ask the human.

## Wisps

Wisps are freeform notes in `.sya/wisps/`. They are not indexed as tasks, have no schema, and cannot be linked from task relations. Use them for ephemeral checklists and raw thoughts that may become tasks later.

```bash
sya wisp create "Release checklist" -d "Draft bullets"
sya wisp list
sya wisp show w-<id>
sya wisp squash w-<id> -t task
sya wisp burn w-<id>
```

Squash creates a real task from the wisp body, then burns the wisp. Burn deletes it without trace.

## Archive And Restore

Archived tasks remain in `.sya/tasks/` with `archived: true`; they still resolve for relations and guards. List/board/ready hide them by default.

Before archive, compact the task body. Replace long implementation logs with a durable summary:

```markdown
## Summary
What was accomplished.

## Key Decisions
- Decision and reason.

## Resolution
How it ended and where to restore detail from.
```

Then archive only terminal tasks:

```bash
sya archive <id>
sya archive --auto
```

`--auto` uses `archive.after_days` from `.sya/config.yml`; until event timestamps exist, created date is the documented age proxy.

Restore historical body from git:

```bash
sya restore <id>
sya restore <id> --at HEAD~2
sya restore <id> --apply
```

`--apply` writes the historical body back while keeping current frontmatter and `archived: true`.

## Completion Convention

When work is done:

1. Move or close the task.
2. Create any `--discovered-from` follow-ups.
3. Run the project quality gates requested by the user.
4. Run `sya doctor`.
5. Commit `.sya/` changes promptly with code changes or as their own commit, following the repository policy. Do not leave board state stranded locally.

If the user has explicitly forbidden commits or pushes, do not commit or push; still keep `.sya/` state correct.
