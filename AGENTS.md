# Agent Instructions

This project uses **sya** (this very tool, dogfooded) for ALL task tracking.
Binary: `sya` (installed in PATH; rebuild+reinstall: `just install && cp ~/go/bin/sya ~/.local/bin/`).

MANDATORY workflow:
1. FIRST: run `sya prime`; for full process semantics run `sya schema docs`.
   Roles, codex-ctl mechanics, audit checklist: `docs/dev-process.md`.
2. Find work: `sya ready`. Claim before working: `sya claim <id>`.
3. Move statuses with `sya move <id> <status>` as you progress. Transitions are
   guarded — when a move is rejected, READ the error: it lists what is missing
   (violations with hints) and what is possible right now (alternatives).
4. Record progress: `sya comment <id> -m "..."`. Specs/designs go into task
   sections: `sya edit <id> --section Description --file -`.
5. Discovered a bug or extra work? `sya create "..." -t bug --rel discovered_from=<current-id>`.
6. Done means done: `sya close <id>` only after the pipeline allows it.
7. Do NOT read or write `.sya/tasks/` directly (`.sya/.ignore` hides the task
   database from grep-style tools on purpose); `.sya/schema.yml`,
   `.sya/config.yml`, and `.sya/memory/` remain readable for agent context.
   Edit `.sya/schema.yml` only when the task is explicitly about changing the
   workflow (validate with `sya schema validate` + `sya doctor` afterwards).
8. Commit `.sya/` changes together with the code they describe; reference the
   task id in the commit message: `feat: ... (sya-<id>)`.

Testing discipline (no CI — everything runs locally):
`PATH=/usr/bin:$PATH just test && just lint` after every change; functional
tier `just func`; integration tier `just integ`. Note: bare `go` may be broken
in interactive zsh (gvm shim) — use `/usr/bin/go`.
