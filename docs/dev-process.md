# Development process

Task tracking lives in **sya itself** (`.sya/`, dogfooding). The pipeline, statuses,
transitions and their conditions are defined in `.sya/schema.yml` — render the
human version any time with `sya schema docs`. This file keeps what the schema
cannot hold: roles, prompts, and codex-ctl mechanics.

## Roles

- **Tech Lead (Claude)**: writes specs (PRD.md sections, task Descriptions),
  reviews deliverables, moves statuses, runs audits, accepts/rejects.
  Does NOT write production code.
- **Developer (Codex via codex-ctl)**: implements, writes tests, fixes bugs.

## Testing tiers (mandatory, in order — locally, no CI)

1. **Unit**: `PATH=/usr/bin:$PATH just test` — table-driven + golden + fuzz seeds.
2. **Functional**: `just func` — black-box CLI against fixtures (exit codes, JSON contracts).
3. **Integration**: `just integ` — real git scenarios (merges, rebases, claim races).

Periodic deep pass: every fuzz target with `-fuzztime 45s+`
(`go test <pkg> -fuzz '^<Target>$' -fuzztime 45s -run xxx`).

## Audit checklist (audit status; RESEARCH task, no edits)

1. **Bugs**: logic errors, edge cases, races, leaks, error-path correctness.
2. **Algorithmic complexity**: no O(N²)+ on task graphs where better is possible.
3. **Code bloat**: dead code, needless abstraction, duplication.
4. **SOLID strict**: S — one concern per package/type; O — guard kinds/relations
   plug in without engine edits; L — fakes honor contracts; I — narrow
   consumer-driven interfaces; D — fs/git behind interfaces injected at main.

## Codex prompt templates

### Implementation
```
Implement <feature>: <description>

CWD: /home/snjax/Documents/projects/devops/sya
Spec: PRD.md §<N> + sya show <task-id> (read both first)
Module: github.com/snjax/sya, Go 1.25+, CGO_ENABLED=0

Requirements:
- cobra commands thin; logic in internal/; fs/git behind interfaces (SOLID/D audited)
- every command supports --json envelope {ok, data|error}; exit codes per PRD §7.2
- PATH=/usr/bin:$PATH just build && just lint must pass
- no git commit/push, no sya status moves beyond your task
```

### Audit
```
RESEARCH TASK - DO NOT EDIT FILES
Audit <path/package> per docs/dev-process.md checklist.
Output: findings (severity, file:line, problem, fix) or "AUDIT CLEAN".
```

## codex-ctl mechanics

```bash
ID=$(codex-ctl spawn "<prompt>" --cwd /home/snjax/Documents/projects/devops/sya | jq -r .session)
codex-ctl state $ID --wait --timeout 600
codex-ctl next $ID                      # read output
codex-ctl act $ID "<follow-up>" enter   # same session = retained context
UUID=$(codex-ctl kill $ID | jq -r .codex_session_id)   # ALWAYS capture
```

Anti-patterns: spawn-per-subtask (use `act`); kill without capturing UUID;
skipping `state --wait`; research prompts without the RESEARCH header.

## Smoke testing

After major milestones: pet project in /tmp driven by a coding agent that must
use sya for all tracking (AGENTS.md pattern). Analyze `.sya/events.jsonl`
(denied transitions = UX signal), agent transcripts, and raw-file discipline
(`.sya/.ignore` keeps grep-based agents out of the tasks database while schema
and config stay observable). Agent not using a feature it should — that's a bug
too. Cheap smokes on Codex, final smokes on Claude subagents.
