# sya — project instructions for Claude

## What this is

`sya` — git-native issue tracker for AI-agent workflows (Go CLI). One task = one
Markdown file with YAML frontmatter under `.sya/`; declarative `schema.yml` with
per-type kanban pipelines; transition guards enforced by the engine (blocking
relations forbid transitions); agent memory (prime/remember); Claude Code plugin.

**Source of truth: `PRD.md`** in repo root (Russian, gitignored). Read the relevant
section before any design or implementation decision. If PRD and code disagree,
PRD wins; if PRD is wrong, fix PRD first (it is the spec).

## Roles

- **Claude = Tech Lead**: writes specs and issue descriptions, reviews deliverables,
  moves bd statuses, runs audits, accepts/rejects. Does NOT write production code.
- **Codex (via codex-ctl) = Developer**: implements, writes tests, fixes bugs.
  Prompt templates and codex-ctl patterns: `.beads/WORKFLOWS.md`.

## Task tracking (bd / beads)

- Pipeline: `open → spec → impl → unit_test → func_test → integ_test → audit → closed`
  (+ `blocked`). Transition rules, backtracks, conditions: **`.beads/WORKFLOWS.md`** —
  follow it strictly; statuses are registered via `status.custom`.
- Start of session: `bd ready` (or `bd prime` output). Claim before working:
  `bd update <id> --claim`.
- Every code commit references its issue: `feat: ... (sya-<id>)`.

## Build & test

```bash
just build      # bin/sya for host
just test       # go test ./... -count=1
just lint       # go vet + gofmt check
just release    # cross-compile linux/darwin × amd64/arm64
```

Go ≥1.25, CGO off. NOTE: in interactive zsh `go` may be shadowed by a broken
gvm shim — use `/usr/bin/go` (and `PATH=/usr/bin:$PATH just ...`) in scripts.

## Quality bar (enforced via audit stage, see WORKFLOWS.md checklist)

- Three mandatory test tiers, in order: unit (table-driven + golden) →
  functional (black-box CLI vs fixtures, JSON contracts, exit codes) →
  integration (real git repos: merges, rebases, conflicts, concurrent claims).
- Audit before close: bugs, algorithmic complexity (no O(N²) on task graphs),
  code bloat, **strict SOLID** (S: one concern per package; O: guard kinds plug in
  without engine edits; L: fakes honor contracts; I: narrow interfaces;
  D: fs/git behind interfaces, injected at main).
- cobra commands stay thin; domain logic in `internal/`; every command has
  `--json` with envelope `{ok, data|error}`; exit codes 0/1/2/3/4 per PRD §7.2.

## Conventions

- Commits: `feat|fix|refactor|test|docs: <description> (sya-<id>)`.
- PRD.md and local notes are gitignored — never commit them.
- `.beads/` data syncs via dolt (`bd dolt push`), not via git commits of dolt files.
- Research/audit prompts to Codex MUST start with "RESEARCH TASK - DO NOT EDIT FILES".
