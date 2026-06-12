---
description: Initialize sya in the current project
argument-hint: "[prefix]"
allowed-tools: Bash(command -v sya), Bash(sya init:*), Bash(sya doctor:*), Bash(pwd), Bash(basename:*)
---

# Initialize sya

Use this when the user wants to start using sya in the current repository.

1. Check whether the `sya` binary is available:

```bash
command -v sya
```

If it is missing, do not continue initialization. Offer this installer command:

```bash
curl -fsSL https://raw.githubusercontent.com/snjax/sya/master/scripts/install.sh | sh
```

2. If `.sya` already exists, report that the project is already initialized and run:

```bash
sya doctor
```

Summarize any findings and stop.

3. Choose a prefix. Use the slash-command argument when present. Otherwise use a sensible default based on the current directory name: lowercase, short, and kebab-like.

4. Run initialization:

```bash
sya init --prefix <prefix>
```

5. Report what was created:

- `.sya/config.yml`
- `.sya/schema.yml`
- `.sya/tasks/`
- `.sya/memory/`
- `.sya/wisps/`
- `.sya/.ignore` and `.sya/.rgignore`, which keep grep-style agent tools out of the tasks database while leaving `schema.yml`, `config.yml`, and `memory/` readable
- `.gitignore` entries for generated sya data, including `wisps/`
- Managed sya integration block in `AGENTS.md`; `CLAUDE.md` too when that file already exists and is not the same file

6. Offer next steps:

- Customize the workflow with the `sya-board` skill when the user wants project-specific task types, statuses, guards, or board columns.
- Create the first epic or task, for example:

```bash
sya create "Initial project setup" -t task
```

- Commit `.sya/` with the initialization changes.
