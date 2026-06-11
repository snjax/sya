---
description: Show sya kanban board columns
argument-hint: [--type T]
---

Run `sya board "$@"` from the current repository.

Use `sya board` for a project-wide board and `sya board --type <type>` for a single type:

```bash
sya board
sya board --type task
```

Archived tasks are hidden by board output. Use `sya list --archived` when the user explicitly asks for archived items.
