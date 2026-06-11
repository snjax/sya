---
name: sya-board
description: >
  Design or revise sya schema.yml boards from a natural-language process:
  statuses, relations, fields, guards, descriptions, docs, validation, and
  migration planning for existing tasks.
allowed-tools: "Read,Bash(sya:*)"
version: "0.1.0"
author: "snjax"
license: "MIT"
---

# sya Board Designer

Use this skill when a user asks to create, revise, or explain a `sya` workflow board. A board is `.sya/schema.yml`: task types, pipelines, statuses, transitions, relations, fields, sections, descriptions, and guards.

## Process

1. Gather a concise process description from the user:
   - task types
   - statuses in order
   - which statuses are working, parked, and terminal
   - allowed children/containers
   - relations and which ones block
   - required fields and body sections
   - guard rules before transitions

2. Write `.sya/schema.yml`.
   - Add `description` at schema level.
   - Add `description` for every type, relation, field, status, and transition.
   - Add `message` and `hint` for every guard.
   - Mark every transition `kind: advance` or `kind: setback`.
   - Use DAG relations for dependencies/provenance when cycles would be invalid.
   - Add `terminal` for each type. Nonterminal statuses must have outgoing transitions.

3. Validate:

```bash
sya schema validate
```

4. Render docs for the human reviewer:

```bash
sya schema docs
sya schema docs --type feature
```

Show the generated docs to the человек for confirmation. They should be able to read the schema as the workflow policy.

5. If tasks already exist:

```bash
sya doctor
```

Then produce a migration plan. Include:

- statuses that need renaming
- tasks whose type/status/field/section no longer conforms
- relation changes and possible dangling refs
- whether a mechanical `sya schema migrate ...` can help, or manual edits are needed

Do not silently rewrite existing tasks without user confirmation.

## Guard Design

Prefer executable guards over prose-only rules. Common patterns:

- `field`: require a frontmatter field value, such as `spec_approved: true`.
- `section_nonempty`: require a body section before moving forward.
- `children_status`: require child tasks in `terminal`.
- `relation_exists`: require a relation before a transition.
- blocking relation: set `blocking: true` on a relation such as `depends_on`; the engine adds an implicit guard that targets must be terminal.

Each guard needs:

```yaml
message: "Why the transition is blocked"
hint: "Concrete command or edit to unblock it"
```

Errors should tell the agent the next step.

## Example 1: Simple Kanban

```yaml
schema_version: 1

description: >
  Simple delivery board for small tasks. Work moves from todo to doing to done.
  Blocked work is represented by dependencies rather than a status.

defaults:
  type: task

relations:
  depends_on:
    description: "Hard work dependency; target must be terminal before source can close."
    reverse: blocks
    graph: dag
    blocking: true
  discovered_from:
    description: "Provenance link for follow-up work discovered while doing another task."
    reverse: discovered
    graph: dag
  relates:
    description: "Non-blocking symmetric association."
    symmetric: true

types:
  task:
    description: "A small unit of work that can be implemented and closed independently."
    pipeline: [todo, doing, done, scrapped]
    statuses:
      todo: "Ready to start."
      doing: "Actively being worked."
      done: "Completed and verified."
      scrapped: "Cancelled with rationale in Log."
    terminal: [done, scrapped]
    working: [doing]
    sections: [Description, Notes]
    fields:
      acceptance:
        type: string
        description: "Short acceptance note for what done means."
    transitions:
      todo -> doing:
        kind: advance
        description: "Start work."
      doing -> done:
        kind: advance
        description: "Work is complete."
        guards:
          - kind: section_nonempty
            section: Description
            message: "Description is empty."
            hint: "Add scope with: sya edit <id> --section Description --file -"
      doing -> todo:
        kind: setback
        description: "Pause work and return it to the queue."
      "* -> scrapped":
        kind: setback
        description: "Cancel the work with a reason in Log."
```

Validation and docs:

```bash
sya schema validate
sya schema docs
```

## Example 2: Dev Pipeline From `.beads/WORKFLOWS.md`

```yaml
schema_version: 1

description: >
  Development workflow with Claude as tech lead and Codex as developer. Work
  starts with specification, moves through implementation and test tiers, then
  reaches audit before closure. Failed review or tests backtrack to impl.

defaults:
  type: task

relations:
  depends_on:
    description: "Blocking dependency; dependency must close before dependent work can close."
    reverse: blocks
    graph: dag
    blocking: true
  discovered_from:
    description: "Follow-up found while working another task."
    reverse: discovered
    graph: dag
  relates:
    description: "Non-blocking relationship between tasks."
    symmetric: true

types:
  epic:
    description: "A container for related implementation tasks."
    container: true
    children: [task, bug, feature, docs]
    pipeline: [open, active, closed, scrapped]
    statuses:
      open: "Epic exists but work has not started."
      active: "Child work is underway."
      closed: "All children are complete and audit is clean."
      scrapped: "Epic cancelled."
    terminal: [closed, scrapped]
    working: [active]
    transitions:
      open -> active:
        kind: advance
        description: "Start the epic."
      active -> closed:
        kind: advance
        description: "Close after all children are terminal."
        guards:
          - kind: children_status
            in: [terminal]
            message: "Epic has non-terminal children."
            hint: "Close or scrap remaining child tasks first."
      "* -> scrapped":
        kind: setback
        description: "Cancel the epic with rationale in Log."

  task:
    description: "Implementation task following the mandatory spec/test/audit pipeline."
    pipeline: [open, spec, impl, unit_test, func_test, integ_test, audit, closed, blocked]
    statuses:
      open: "Task created, not yet started."
      spec: "Claude writes or reviews the specification."
      impl: "Codex implements; code compiles and lint is clean."
      unit_test: "Unit tests are being written or run."
      func_test: "Functional black-box CLI tests are being run."
      integ_test: "Real git integration scenarios are being run."
      audit: "Strict code audit for bugs, complexity, bloat, and SOLID."
      closed: "Done; audit clean and project handoff complete."
      blocked: "Waiting for an external blocker."
    terminal: [closed]
    working: [spec, impl, unit_test, func_test, integ_test, audit]
    parked: [blocked]
    sections: [Description, Design, Acceptance, Notes]
    fields:
      tests_green:
        type: bool
        default: false
        description: "True after requested quality gates pass."
      audit_clean:
        type: bool
        default: false
        description: "True when audit has no findings."
    transitions:
      open -> spec:
        kind: advance
        description: "Work begins; Claude writes or reviews spec."
      spec -> impl:
        kind: advance
        description: "Spec approved; implementation starts."
        guards:
          - kind: section_nonempty
            section: Design
            message: "Design section is empty."
            hint: "Add design notes with: sya edit <id> --section Design --file -"
      impl -> unit_test:
        kind: advance
        description: "Implementation is complete enough for unit tests."
      unit_test -> func_test:
        kind: advance
        description: "Unit tests are green; run functional tests."
        guards:
          - kind: field
            field: tests_green
            equals: true
            message: "Tests are not marked green."
            hint: "Run tests, then: sya update <id> --field tests_green=true"
      func_test -> integ_test:
        kind: advance
        description: "Functional tests are green; run integration tests."
      integ_test -> audit:
        kind: advance
        description: "Integration tests are green; start audit."
      audit -> closed:
        kind: advance
        description: "Audit is clean; close the task."
        guards:
          - kind: field
            field: audit_clean
            equals: true
            message: "Audit has not been marked clean."
            hint: "Complete audit, then: sya update <id> --field audit_clean=true"
      unit_test -> impl:
        kind: setback
        description: "Unit test failure; fix implementation."
        ignore_blocking: [depends_on]
      func_test -> impl:
        kind: setback
        description: "Functional failure; fix implementation."
        ignore_blocking: [depends_on]
      integ_test -> impl:
        kind: setback
        description: "Integration failure; fix implementation."
        ignore_blocking: [depends_on]
      audit -> impl:
        kind: setback
        description: "Audit finding; return to implementation."
        ignore_blocking: [depends_on]
      "* -> blocked":
        kind: setback
        description: "External blocker discovered."
      blocked -> open:
        kind: setback
        description: "Blocker resolved; return to open if exact prior status is unknown."
```

After writing this schema:

```bash
sya schema validate
sya schema docs
sya doctor
```

If existing tasks use old statuses, prepare a migration plan before editing files.
