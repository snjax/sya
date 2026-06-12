# sya

`sya` is a git-native issue tracker for AI-agent workflows. Project state lives
under `.sya/`; one task is one Markdown file with YAML frontmatter. Workflows are
schema-driven, so each task type can have its own pipeline, fields, sections,
relations, and terminal/working states. Transitions are enforced by the engine:
schema guards and blocking relations reject invalid moves with actionable human
or JSON errors. There is no SQL database, daemon, or network service in the core
CLI. Git is the storage, review, and merge layer.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/snjax/sya/master/scripts/install.sh | sh
```

The installer requires Go 1.26 or newer, runs `go install`, copies the binary to
`~/.local/bin`, and prints `sya version`. It does not use sudo, git, or just.

Overrides:

```bash
curl -fsSL https://raw.githubusercontent.com/snjax/sya/master/scripts/install.sh | SYA_INSTALL_DIR="$HOME/bin" sh
curl -fsSL https://raw.githubusercontent.com/snjax/sya/master/scripts/install.sh | SYA_VERSION="latest" sh
curl -fsSL https://raw.githubusercontent.com/snjax/sya/master/scripts/install.sh | SYA_VERSION="v0.1.0" SYA_INSTALL_DIR="$HOME/bin" sh
```

Alternative for users who want Go to place the binary in `GOBIN` or
`$(go env GOPATH)/bin` directly:

```bash
go install github.com/snjax/sya/cmd/sya@latest
```

## Quick Start

```bash
sya init --prefix myproj
```

```bash
sya create "Wire SSE transport" -t task -p high
sya ready
sya move a3f8c1 in_progress
sya close a3f8c1 --reason "implemented"
```

Guarded transitions fail with next steps:

```bash
sya create "Streaming responses" -t feature
sya move b771d2 spec
sya move b771d2 impl
# error: transition blocked
```

With `--json`, the same failure is machine-readable and includes guard hints:

```json
{"ok":false,"error":{"type":"transition_blocked","message":"transition blocked","task":"b771d2","violations":[{"kind":"field","field":"spec_approved","message":"Spec is not approved","hint":"sya update <id> --field spec_approved=true"},{"kind":"section_nonempty","section":"Design","message":"Design is empty","hint":"sya edit <id> --section Design"}],"alternatives":[{"to":"scrapped","kind":"setback"}]}}
```

## Schema

`.sya/schema.yml` defines task types, relations, transition whitelists, and
guards. Short example:

```yaml
schema_version: 1
defaults:
  type: task

relations:
  depends_on:
    reverse: blocks
    graph: dag
    blocking: true

types:
  task:
    pipeline: [todo, in_progress, done, scrapped]
    terminal: [done, scrapped]
    working: [in_progress]
    transitions:
      todo -> in_progress: {}
      in_progress -> done: {}
      "* -> scrapped": {}

  feature:
    pipeline: [draft, spec, impl, review, done, scrapped]
    terminal: [done, scrapped]
    working: [impl, review]
    sections: [Description, Design, Acceptance]
    fields:
      spec_approved: {type: bool, default: false}
    transitions:
      draft -> spec: {}
      spec -> impl:
        guards:
          - kind: field
            field: spec_approved
            equals: true
            message: "Spec is not approved"
            hint: "sya update <id> --field spec_approved=true"
```

```bash
sya schema docs
sya schema graph --type feature
```

## Agent Integration

```text
/plugin marketplace add snjax/sya
```

The plugin hook pattern is to run `sya prime` on session start and before
compaction. `sya prime` prints compact context: schema digest, ready tasks,
in-progress tasks, and memory note index. Outside a sya project it exits 0 with
no output, so global hooks stay quiet in unrelated repositories.

For Codex and OpenCode, use the same pattern in `AGENTS.md`:

```markdown
Run `sya prime` before starting work.
Use `sya ready`, `sya show <id>`, `sya move`, and `sya doctor`.
Use `--json` when automation needs stable envelopes and exit codes.
```

```bash
sya remember "Deploys require green doctor" --key deploy-process --task a3f8c1
sya recall deploy-process
```

## JSON API

All commands support `--json` with a stable stdout envelope:

```json
{"ok":true,"data":{}}
{"ok":false,"error":{"type":"usage","message":"..."}}
```

Diagnostics go to stderr. Exit codes and error types are documented in
[docs/json-api.md](docs/json-api.md).

## Comparison

| Requirement | beads | beans | sya |
| --- | --- | --- | --- |
| Text git-friendly storage | No, Dolt-backed; JSONL fallback is awkward for merge | Yes, Markdown | Yes, Markdown tasks |
| Per-type kanban columns | No, global statuses | No, hardcoded | Yes, schema-driven |
| Engine-enforced transitions | No, workflow convention | No | Yes |
| Relations that block transitions | Partly: `blocks` affects ready, not moves | No | Yes, by default |
| Epics | Yes | Yes | Yes |
| Agent memory / prime | Yes | Partial | Yes |
| Workflow/schema builder path | Partial via workflow setup | No | Yes, schema-first |
| Project direction | External roadmap | Contributions not accepted | This repo |

`sya` borrows useful semantics from beads and the file-per-task model from beans,
then adds a schema engine with guards.

## Build and Test

```bash
just build
just test
just lint
just install
just func
just integ
just release
```
