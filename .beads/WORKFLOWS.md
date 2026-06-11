# Workflow

## Roles

- **Tech Lead (Claude)**: Writes specs (PRD.md sections, issue descriptions), reviews deliverables, moves statuses, accepts/rejects work, runs audits. Does NOT write code directly — delegates to Codex.
- **Developer (Codex via codex-ctl)**: Implements code, writes tests, fixes bugs. Receives task prompts from Tech Lead. Runs via `codex-ctl spawn` in full-auto mode.

## Statuses

| Status | Description |
|--------|-------------|
| open | Task created, not yet started |
| spec | Writing or reviewing specification / design (PRD section, interface sketch) |
| impl | Implementation in progress (Codex working); compiles, `just lint` clean |
| unit_test | Writing and running unit tests (`go test ./...`, table-driven + golden files) |
| func_test | Functional black-box CLI tests (built binary against fixture projects, golden output) |
| integ_test | Integration tests: end-to-end git scenarios (branches, merge/rebase conflicts, hooks, concurrent claims) |
| audit | Code audit: bugs, algorithmic complexity, code bloat, strict SOLID review |
| blocked | Waiting on dependency or external blocker |
| closed | Done — audit clean, code committed and pushed |

## Transitions

```
open ──► spec ──► impl ──► unit_test ──► func_test ──► integ_test ──► audit ──► closed
                   ▲          │              │              │            │
                   │          ▼              ▼              ▼            ▼
                   └──────────┴──────────────┴──────────────┴────────────┘
                                  (backtrack to impl)

* ──► blocked ──► * (return to previous status)
impl ──► spec (rare: spec error discovered)
```

| From | To | Condition |
|------|-----|-----------|
| open | spec | Work begins — Claude writes/reviews spec |
| spec | impl | Spec approved — Codex starts implementing |
| impl | unit_test | Implementation complete — compiles, `just lint` clean |
| unit_test | func_test | All unit tests pass (`go test ./... -count=1`) |
| func_test | integ_test | All functional CLI tests pass |
| integ_test | audit | All integration (git e2e) tests pass |
| audit | closed | Audit clean (bugs, complexity, bloat, SOLID) — commit and push |
| impl | spec | Backtrack: implementation reveals spec error or impossibility |
| unit_test | impl | Backtrack: unit test failures require code fix |
| func_test | impl | Backtrack: functional test failures require code fix |
| integ_test | impl | Backtrack: e2e/git-scenario failures require code fix |
| audit | impl | Backtrack: audit finds bugs, complexity, bloat, or SOLID violations |
| * | blocked | Blocker discovered |
| blocked | * | Blocker resolved — return to previous status |

## Backtrack Rules

1. **spec <- impl**: Rare. Only when spec has an error or is impossible. Default: spec is correct, fix implementation.
2. **impl <- unit_test**: Test failure means code bug. Codex fixes code, re-runs tests.
3. **impl <- func_test**: CLI behaves wrong as a black box. Codex fixes, re-runs unit + functional.
4. **impl <- integ_test**: Git-scenario failure means a real-world bug (merge, concurrency). Codex fixes, re-runs all tiers.
5. **impl <- audit**: Audit findings (see checklist below). Codex fixes, Claude re-audits. Tests must stay green.

## Testing Tiers (mandatory, in order)

Never skip a tier. Each tier runs only after the previous one is green.

1. **Unit tests**: `go test ./... -count=1 -timeout 120s` — table-driven; golden files for parser/serializer/schema-engine; guard-engine truth tables.
2. **Functional tests**: black-box CLI — run the built `bin/sya` against fixture `.sya/` projects in temp dirs; verify stdout/stderr/exit codes against goldens (testscript-style). Cover every command and every error contract from docs/json-api.md (`transition_blocked`, `transition_not_allowed`, `offending`, `alternatives`, exit codes 0-4).
3. **Integration tests**: end-to-end git scenarios — real `git` repos in temp dirs: parallel branches creating/editing tasks, merge/rebase with Log conflicts (`doctor --fix-merge`), dangling refs after merge, duplicate id repair (`--reassign-id`), schema evolution across branches, concurrent `claim` races (same worktree), `sya prime` hook behavior in foreign repos.

Rationale: unit catches logic bugs instantly; functional pins the agent-facing CLI contract; integration validates the git-native promises (the riskiest part of sya).

## Audit Checklist (audit status)

Audit is a RESEARCH task (no edits). Verdict: clean / findings list with file:line.

1. **Bugs**: logic errors, edge cases (empty sets, vacuous guards, prefix-id ambiguity), race conditions (claim CAS, cache writes), resource leaks, error-path correctness.
2. **Algorithmic complexity**: no O(N²) or worse where O(N log N)/O(N) is possible — graph traversals (ready/blocked, dag checks), index rebuild, guard evaluation. Justify any hot-path allocation churn.
3. **Code bloat**: dead code, unnecessary abstractions, premature generalization, over-engineering, copy-paste that should be a helper (and helpers that should be inlined).
4. **SOLID (strict)**:
   - **S**: one reason to change per package/type — parser, schema engine, index, CLI commands are separate concerns; no god-packages.
   - **O**: guard kinds and relation semantics extensible without modifying the engine core (new kind = new file, registered, not a switch edited in five places).
   - **L**: implementations honor interface contracts (Storage/Index fakes in tests must behave like the real thing).
   - **I**: small consumer-driven interfaces; commands depend on the narrow capability they use, not a fat Store.
   - **D**: cmd layer depends on abstractions; filesystem/git access behind interfaces injected at the edge (`main`), never imported deep in domain logic.

Do NOT audit for: missing features, style nits covered by gofmt/lint, speculative future needs.

## Commit Protocol

When a task reaches `closed`:
1. All three test tiers pass locally.
2. Audit verdict is clean.
3. `just lint` and `gofmt` clean.
4. Commit and push:
   ```bash
   git add <changed files>
   git commit -m "feat|fix|refactor|test: <description> (sya-<id>)"
   git push
   ```
5. `bd close <id> --reason "<short summary>"`.

## Architecture (orientation for prompts)

- **Spec**: PRD.md in repo root (Russian, gitignored) — source of truth for all design decisions.
- **Module**: `github.com/snjax/sya`, Go ≥1.25, CGO off. CLI entry: `cmd/sya`.
- **Planned layout**: `internal/task` (file model, frontmatter), `internal/schema` (parse/validate/guards), `internal/index` (in-memory + cache), `internal/gitx` (git helpers), `cmd/sya` (cobra commands, thin).
- **Build**: `just build|test|lint|release` (release: linux/darwin × amd64/arm64).
- **Contracts**: JSON envelope `{ok, data|error}`; exit codes 0/1/2/3/4; errors carry `violations` (message+hint+offending) and `alternatives`/`allowed`.

## Developer (Codex) Task Prompt Templates

### Implementation
```
Implement <feature>: <description>

CWD: /home/snjax/Documents/projects/devops/sya
Spec: PRD.md §<N> (read it first; it is the source of truth)
Module: github.com/snjax/sya, Go 1.25+, CGO_ENABLED=0

Requirements:
- Follow existing package layout and code patterns
- cobra commands stay thin; logic lives in internal/
- All I/O (fs, git) behind interfaces injected from main — SOLID/D is audited strictly
- Every command supports --json with envelope {ok, data|error}; exit codes per PRD §7.2
- Build must pass: just build && just lint
- Do not write tests in this task unless asked — separate unit_test stage follows
```

### Unit Tests
```
Write unit tests for <package/path>

CWD: /home/snjax/Documents/projects/devops/sya
Run: go test ./... -count=1 -timeout 120s
Style: table-driven; golden files under testdata/ for parser/serializer/schema cases
Cover: happy paths, error paths, edge cases (empty relations, vacuous guards,
wildcard expansion, terminal statuses, prefix-id ambiguity)
No network, no global state; use t.TempDir()
```

### Functional Tests (CLI black-box)
```
Write functional tests for <command(s)>

CWD: /home/snjax/Documents/projects/devops/sya
Approach: build bin/sya, execute against fixture .sya/ projects in t.TempDir(),
assert stdout/stderr/exit codes against goldens (update with -update flag)
Must cover: --json envelope shape, error contracts (transition_blocked with
violations/offending/alternatives, transition_not_allowed with allowed),
exit codes 0/1/2/3/4
```

### Integration Tests (git e2e)
```
Write integration tests for <scenario>

CWD: /home/snjax/Documents/projects/devops/sya
Approach: real git repos in t.TempDir() (git init, branches, merges)
Scenarios: parallel task creation in branches (no conflicts expected),
same-task edits in two branches (Log-only conflict -> doctor --fix-merge),
dangling refs after merge -> doctor detects, duplicate ids -> --reassign-id,
schema changed in one branch + tasks in another, concurrent claim (two
processes, one must get already_claimed)
```

### Audit
```
RESEARCH TASK - DO NOT EDIT FILES
Audit <path/package>

Check, with file:line references:
1. Bugs: logic errors, edge cases, races, leaks, error-path correctness
2. Algorithmic complexity: O(N^2)+ where better is possible (graph walks, index, guards)
3. Code bloat: dead code, needless abstraction, over-engineering, duplication
4. SOLID strict:
   S - one concern per package/type; O - new guard kinds/relations plug in
   without editing engine core; L - fakes honor contracts; I - narrow
   consumer-driven interfaces; D - fs/git behind interfaces, injected at main
Do NOT report: missing features, gofmt/lint-covered style, speculative needs.
Output: structured findings list (severity, file:line, problem, suggested fix)
or explicit "AUDIT CLEAN".
```

## codex-ctl Usage Patterns

### Spawn for implementation
```bash
ID=$(codex-ctl spawn "<prompt>" --cwd /home/snjax/Documents/projects/devops/sya | jq -r .session)
codex-ctl state $ID --wait --timeout 600
codex-ctl last $ID
```

### Follow up on same session (preserves context — default for same workstream)
```bash
codex-ctl act $ID "<follow-up task>" enter
codex-ctl state $ID --wait --timeout 300
codex-ctl next $ID
```

### Handle stuck prompts
```bash
codex-ctl screen $ID          # see what's on screen
codex-ctl act $ID enter       # approve trust/permission prompt
codex-ctl act $ID y enter     # approve yes/no prompt
```

### Kill and capture UUID for resume (ALWAYS capture)
```bash
UUID=$(codex-ctl kill $ID | jq -r .codex_session_id)
# later: ID=$(codex-ctl spawn --resume $UUID "new task" | jq -r .session)
```

### Anti-patterns
- **Don't spawn-per-subtask** — one session per workstream, follow up via `act`
- **Don't kill without capturing UUID** — session context is lost forever
- **Don't skip `state --wait`** — always wait for idle before reading output
- **Research tasks MUST start with "RESEARCH TASK - DO NOT EDIT FILES"** — otherwise Codex starts implementing
