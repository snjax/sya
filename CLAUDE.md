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

- **Claude = Tech Lead**: writes specs and task descriptions, reviews deliverables,
  moves statuses, runs audits, accepts/rejects. Does NOT write production code.
- **Codex (via codex-ctl) = Developer**: implements, writes tests, fixes bugs.
  Prompt templates and codex-ctl patterns: `docs/dev-process.md`.

## Task tracking (sya — dogfooded)

- Tracker: `.sya/` in this repo, managed with the installed `sya` binary.
- Pipeline for tasks: `open → spec → impl → unit_test → func_test → integ_test → audit → done`;
  bugs: `open → impl → verify → done`. Full semantics with descriptions and
  guard conditions: `sya schema docs` (source: `.sya/schema.yml`).
- Session start: `sya prime`. Find work: `sya ready`. Claim: `sya claim <id>`.
- Move statuses honestly; rejected transitions explain themselves (violations,
  hints, alternatives). Discovered work: `--rel discovered_from=<id>`.
- Commit `.sya/` changes together with the code; reference the task:
  `feat: ... (sya-<id>)`.
- Do not edit files under `.sya/` directly (only `schema.yml`, followed by
  `sya schema validate` + `sya doctor`).

## Build & test

```bash
just build      # bin/sya for host
just test       # go test ./... -count=1 (unit + functional + integration)
just func       # functional tier only
just integ      # integration tier only
just lint       # go vet + gofmt check
just release    # cross-compile linux/darwin × amd64/arm64
just install    # install to GOBIN; then: cp ~/go/bin/sya ~/.local/bin/
```

Go ≥1.25, CGO off. No CI — run everything locally after every change.
NOTE: in interactive zsh `go` may be shadowed by a broken gvm shim — use
`/usr/bin/go` (and `PATH=/usr/bin:$PATH just ...`) in scripts.

## Quality bar (enforced via audit stage, checklist in docs/dev-process.md)

- Three mandatory test tiers, in order: unit (table-driven + golden) →
  functional (black-box CLI vs fixtures, JSON contracts, exit codes) →
  integration (real git repos: merges, rebases, conflicts, concurrent claims).
  Periodic fuzz passes over all Fuzz targets.
- Audit before done: bugs, algorithmic complexity (no O(N²) on task graphs),
  code bloat, **strict SOLID** (S: one concern per package; O: guard kinds plug in
  without engine edits; L: fakes honor contracts; I: narrow interfaces;
  D: fs/git behind interfaces, injected at main).
- cobra commands stay thin; domain logic in `internal/`; every command has
  `--json` with envelope `{ok, data|error}`; exit codes 0/1/2/3/4 per PRD §7.2.

## Conventions

- Commits: `feat|fix|refactor|test|docs: <description> (sya-<id>)`.
- PRD.md and local notes are gitignored — never commit them.
- Research/audit prompts to Codex MUST start with "RESEARCH TASK - DO NOT EDIT FILES".
- Smoke tests after milestones: pet projects in /tmp driven by agents that must
  use sya; analyze events.jsonl and transcripts; agent misuse of sya = bug.
