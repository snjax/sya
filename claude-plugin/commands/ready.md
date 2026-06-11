---
description: Show ready sya tasks with no blocking guards
argument-hint: [--type T] [--label L] [--assignee A] [--limit N]
---

Run `sya ready "$@"` from the current repository.

Use this when the user asks what can be worked on next. If the command returns tasks, show ID, type, status, priority, assignee, and title. If the user chooses one, claim it with:

```bash
sya claim <id>
```

Useful filters:

```bash
sya ready --type feature --limit 5
sya ready --label cli --assignee codex
```
