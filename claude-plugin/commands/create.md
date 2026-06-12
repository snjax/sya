---
description: Create a sya task
argument-hint: '"Title" [-t type] [-p priority] [--parent id] [--depends-on id]'
---

Create a task with `sya create`.

If the user gives only a title, run:

```bash
sya create "Task title"
```

Use concrete flags when the user provides structure:

```bash
sya create "Implement streaming" -t feature -p high --parent <epic-id> --depends-on <task-id> -d "Initial notes"
sya create "Bug title" -t bug --field severity=critical
sya create "Follow-up" --discovered-from <current-id>
```

For multiple tasks from YAML on stdin:

```bash
sya create --from-file -
```
