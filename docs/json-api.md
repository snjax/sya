# sya JSON API

This document defines the M1 CLI JSON contract from PRD 7.1 and 7.2.
The exact error envelopes are pinned by golden files in `internal/syaerr/testdata`.

## Streams

- Command data is written to stdout.
- Diagnostics and progress are written to stderr.
- With `--json`, both success and error envelopes are written to stdout. Human diagnostics are not mixed into JSON stdout.

## Success Envelope

Every successful JSON response is an object with `ok: true` and a command-specific `data` value.

```json
{"ok":true,"data":{"version":"test"}}
```

Commands that intentionally produce no data may omit `data`.

## Error Envelope

Every JSON error response is an object with `ok: false` and an `error` object. The `type` field is stable within a major version.

```json
{"ok":false,"error":{"type":"usage","message":"missing task id"}}
```

Single-id mutation commands such as `move`, `close`, and `claim` return this top-level error envelope when the operation fails. Multi-id mutation commands return `ok: true` with per-id `data.results` and exit 3 when at least one id fails, so callers can inspect successes and failures from the same request.

## Exit Codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Usage error, or an uncoded internal error |
| 2 | Lookup failure: not found or ambiguous id prefix |
| 3 | Transition rejected: whitelist, guard, or claim rejection |
| 4 | Invalid schema or task file |

Partial success in multi-id commands exits 3 and returns per-id results in `data.results`.

## Error Types

### usage

Exit code: 1.

```json
{"ok":false,"error":{"type":"usage","message":"missing task id"}}
```

### not_found

Exit code: 2.

```json
{"ok":false,"error":{"type":"not_found","message":"not found: a3f8c1","id":"a3f8c1"}}
```

### ambiguous

Exit code: 2. `candidates` contains the matching task summaries.

```json
{"ok":false,"error":{"type":"ambiguous","message":"ambiguous prefix: a3","prefix":"a3","candidates":[{"id":"a3f8c1","title":"Streaming responses","type":"feature","status":"impl","file":".sya/tasks/a3f8c1-streaming-responses.md"},{"id":"a3b771","title":"Retry transport","type":"bug","status":"todo","file":".sya/tasks/a3b771-retry-transport.md"}]}}
```

### transition_not_allowed

Exit code: 3. The requested transition is not in the schema whitelist. `allowed` lists transitions that are valid from the current status.

```json
{"ok":false,"error":{"type":"transition_not_allowed","message":"transition not allowed","task":"a3f8c1","from":"draft","to":"done","allowed":[{"to":"spec","kind":"advance","description":"Requirements are ready for specification"},{"to":"scrapped","kind":"setback","description":"Task was cancelled with rationale in Log"}]}}
```

### transition_blocked

Exit code: 3. The transition is whitelisted, but one or more guards failed. `violations` explains why the transition is blocked; each violation may include a `hint` and `offending` task summaries. `alternatives` lists currently passable transitions from the same status.

```json
{"ok":false,"error":{"type":"transition_blocked","message":"transition blocked","task":"a3f8c1","transition":{"from":"spec","to":"impl","kind":"advance","description":"Specification approved; start implementation"},"violations":[{"kind":"field","field":"spec_approved","message":"Spec is not approved (fields.spec_approved)","hint":"After spec review: sya update a3f8c1 --field spec_approved=true"},{"kind":"blocking_relation","relation":"depends_on","message":"Dependencies are not closed","offending":[{"id":"b771d2","title":"Transport spike","type":"task","status":"impl","file":".sya/tasks/b771d2-transport-spike.md"}]}],"alternatives":[{"to":"scrapped","kind":"setback","description":"Task was cancelled with rationale in Log"}]}}
```

### claim_not_reachable

Exit code: 3. `claim` uses this when the task type has working statuses, but the current status has no direct whitelist transition into any working status. `working` lists the claim targets for the type, and `next_advance` is the first currently passable advance transition from the current status, or `null` if none exists.

```json
{"ok":false,"error":{"type":"claim_not_reachable","message":"cannot claim: working statuses for feature are impl, review; no transition from draft; advance first: sya move a3f8c1 spec","task":"a3f8c1","task_type":"feature","from":"draft","working":["impl","review"],"next_advance":{"to":"spec","kind":"advance","description":"Requirements are ready for specification"}}}
```

### schema_invalid

Exit code: 4. `violations` contains schema or task-file validation findings.

```json
{"ok":false,"error":{"type":"schema_invalid","message":"schema validation failed","violations":[{"kind":"schema","file":".sya/schema.yml","message":"types.feature.terminal is required","hint":"Add at least one terminal status"}]}}
```

### conflict_markers

Exit code: 4. This is a schema/file validity error for unresolved merge conflict markers.

```json
{"ok":false,"error":{"type":"conflict_markers","message":".sya/tasks/a3f8c1-streaming-responses.md: conflict markers found","path":".sya/tasks/a3f8c1-streaming-responses.md"}}
```
