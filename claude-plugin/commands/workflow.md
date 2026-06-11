---
description: Show the sya agent workflow guide
---

# sya Workflow

1. Start with `sya prime`. The plugin runs it on `SessionStart` and `PreCompact`; it exits silently outside sya projects.
2. Find work with `sya ready` or inspect the board with `sya board`.
3. Claim work with `sya claim <id>`.
4. Read details with `sya show <id>` and inspect allowed moves with `sya transitions <id>`.
5. Move status with `sya move <id> <status>` or close terminal work with `sya close <id> --reason "done"`.
6. When new work is discovered, create it with `sya create "Title" --discovered-from <id>`.
7. Update task text with `sya edit <id> --section <Name> --file -` or add short notes with `sya comment <id> -m "message"`.
8. Before archiving, summarize the body into `Summary`, `Key Decisions`, and `Resolution`; then run `sya archive <id>`.
9. Commit `.sya/` changes promptly according to the project convention.

Guard errors are instructions. In JSON output, read:

- `violations`: what failed.
- `offending`: related tasks that must be fixed first.
- `hint`: the exact suggested command or action.
- `alternatives`: currently passable transitions.
- `allowed`: valid whitelist transitions when the requested move is not allowed.
