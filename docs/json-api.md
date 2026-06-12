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

## Success Data Examples

### create

```json
{"ok":true,"data":{"id":"a3f8c1","file":".sya/tasks/a3f8c1-streaming.md","relations":{"depends_on":["b771d2"]}}}
```

### show

`show` returns a full task card, computed relations, body sections, optional memory refs, quarantine warnings, and frontmatter links. With `--thread`, `thread` is an ordered tree array for the `discovered_from` chain.

```json
{"ok":true,"data":{"task":{"id":"a3f8c1","type":"feature","title":"Streaming","status":"impl","status_description":"Implementation in progress","links":[{"url":"https://example.test/pr/1","title":"PR"}],"file":".sya/tasks/a3f8c1-streaming.md"},"relations":{"depends_on":["b771d2"],"discovered":["c22222"]},"thread":[{"id":"b00000","type":"task","title":"Origin","status":"done","file":".sya/tasks/b00000-origin.md","depth":0,"direction":"ancestor"},{"id":"a3f8c1","type":"feature","title":"Streaming","status":"impl","file":".sya/tasks/a3f8c1-streaming.md","depth":1,"direction":"current","current":true}],"sections":[{"name":"Description","text":"..."}]}}
```

### list, ready, blocked

`list` and `ready` return `{"tasks":[...]}` with compact task summaries. `blocked` returns `{"tasks":[{"task":...,"dead_end":false,"transitions":[...]}]}`.

```json
{"ok":true,"data":{"tasks":[{"id":"a3f8c1","type":"feature","title":"Streaming","status":"impl","priority":"high","assignee":"codex","file":".sya/tasks/a3f8c1-streaming.md"}]}}
```

### query

`query <expr>` returns the same compact task summary schema as `list`. Supported flags are `--archived` and `--limit`.

```json
{"ok":true,"data":{"tasks":[{"id":"a3f8c1","type":"feature","title":"Streaming","status":"impl","priority":"high","parent":"e00001","assignee":"codex","labels":["cli"],"file":".sya/tasks/a3f8c1-streaming.md"}]}}
```

Example:

```bash
sya --json query 'type=feature and not terminal and (age>7d or blocked)' --limit 10
```

### stats

`stats` returns per-type status counts, project totals, quarantine/wisp counts, and active-task age stats.

```json
{"ok":true,"data":{"types":[{"type":"task","statuses":[{"status":"todo","count":3},{"status":"in_progress","count":1},{"status":"done","count":8}],"total":12}],"totals":{"active":4,"terminal":8,"archived":2,"ready":3,"blocked":1,"dead_end":0,"tasks":14},"quarantined":0,"wisps":2,"age":{"active_average_days":3.5,"active_max_days":9}}}
```

### stale

`stale` returns active tasks whose latest Log entry, or created time if no Log exists, is older than `--days`. Supported flags are `--days`, `--type/-t`, and `--limit`.

```json
{"ok":true,"data":{"tasks":[{"id":"a3f8c1","type":"feature","status":"impl","title":"Streaming","days_stale":17,"file":".sya/tasks/a3f8c1-streaming.md"}]}}
```

### duplicates

`duplicates` returns similar task pairs. Supported flags are `--threshold`, `--all`, and `--limit`.

```json
{"ok":true,"data":{"pairs":[{"a":{"id":"a3f8c1","type":"feature","title":"Streaming responses","status":"impl","file":".sya/tasks/a3f8c1-streaming-responses.md"},"b":{"id":"b771d2","type":"task","title":"Streaming transport","status":"todo","file":".sya/tasks/b771d2-streaming-transport.md"},"score":0.812,"hint":"sya link a3f8c1 duplicates b771d2"}]}}
```

### transitions

```json
{"ok":true,"data":{"task":"a3f8c1","status":"spec","transitions":[{"to":"impl","kind":"advance","description":"Start implementation","target_status_description":"Implementation in progress","passing":false,"violations":[{"kind":"section_nonempty","section":"Design","file":"/repo/.sya/tasks/a3f8c1-streaming.md","message":"Design is empty"}]},{"to":"scrapped","kind":"setback","passing":true}]}}
```

`check` and `attest` guards can appear as deferred violations in `transitions` and `blocked` payloads:

- `{"kind":"check","deferred":true,...}` means the command was inspected without executing checks, such as through `sya transitions` or `sya move --explain`.
- `{"kind":"attest","deferred":true,"attest_id":"review","question":"...","hint":"--attest review=\"yes: <justification>\""}` means the transition needs an explicit attestation before a mutation command can pass.
- `move`, `close`, and `claim` accept repeated `--attest id="yes: <justification>"` flags. The answer must start with `yes:` and include a nontrivial justification. Successful attestations are written to Log and events.

```json
{"ok":true,"data":{"task":"a3f8c1","status":"review","transitions":[{"to":"done","kind":"advance","description":"Review passed","passing":false,"violations":[{"kind":"attest","message":"Human review must be confirmed","hint":"--attest human_review=\"yes: <justification>\"","deferred":true,"question":"Did a human reviewer approve the result?","attest_id":"human_review"}]}]}}
```

### prime

```json
{"ok":true,"data":{"project":{"name":"sya","prefix":"sya","root":"/repo"},"schema":{"types":[{"name":"task","pipeline":["todo","in_progress*","done!"]}],"relations":[{"name":"depends_on","reverse":"blocks","graph":"dag","blocking":true}]},"ready":[],"in_progress":[],"memory":[]}}
```

### template list

```json
{"ok":true,"data":{"templates":[{"name":"feature-set","description":"Create a feature with spec and follow-up.","path":".sya/templates/feature-set.yml"}]}}
```

### template show

```json
{"ok":true,"data":{"template":{"name":"feature-set","description":"Create a feature with spec and follow-up.","params":[{"name":"name","required":true,"description":"Feature name"}],"tasks":[{"key":"spec","type":"docs","title":"Spec {{name}}","sections":{"Description":"Spec for {{name}}"}},{"key":"impl","type":"feature","title":"Build {{name}}","relations":{"depends_on":["spec"]}}],"path":".sya/templates/feature-set.yml"}}}
```

### template apply

`template apply <name>` returns a key-to-created-task map. `--dry-run` uses the same schema with `dry_run:true` and performs no writes. Supported flags are `--param/-p`, `--parent`, and `--dry-run`.

```json
{"ok":true,"data":{"template":"feature-set","tasks":{"spec":{"id":"a00001","file":".sya/tasks/a00001-spec-streaming.md","type":"docs","title":"Spec Streaming","parent":"e00001"},"impl":{"id":"b00001","file":".sya/tasks/b00001-build-streaming.md","type":"feature","title":"Build Streaming","parent":"e00001","relations":{"depends_on":["a00001"]}}}}}
```

Dry-run:

```json
{"ok":true,"data":{"template":"feature-set","dry_run":true,"tasks":{"spec":{"id":"a00001","file":".sya/tasks/a00001-spec-streaming.md","type":"docs","title":"Spec Streaming"}}}}
```

### graph

`graph` renders the instance graph. By default it includes blocking relations only. Supported flags are `--rel`, `--all-relations`, `--epic`, `--type/-t`, and `--format mermaid|dot`.

```json
{"ok":true,"data":{"format":"mermaid","content":"flowchart TD\n  n_a3f8c1[\"a3f8c1 Streaming [impl]\"]\n  n_a3f8c1 -->|depends_on| n_b771d2","nodes":[{"id":"a3f8c1","title":"Streaming","type":"feature","status":"impl","parent":"e00001"},{"id":"b771d2","title":"Transport spike","type":"task","status":"done","archived":true}],"edges":[{"from":"a3f8c1","to":"b771d2","relation":"depends_on"}]}}
```

### roadmap

`roadmap` returns a deterministic Markdown roadmap and the structured data used to render it. `-o/--output` writes the Markdown to a file and sets `written`.

```json
{"ok":true,"data":{"project":"sya","epics":[{"task":{"id":"e00001","type":"epic","title":"M2","status":"active","file":".sya/tasks/e00001-m2.md"},"description":"CLI milestone","closed":2,"total":5,"bar":"[████░░░░░░]","children":[{"task":{"id":"a3f8c1","type":"feature","title":"Streaming","status":"impl","assignee":"codex","file":".sya/tasks/a3f8c1-streaming.md"},"icon":"⛔","blocked":true,"blocked_reason":"blocking relation \"depends_on\" has non-terminal targets"}]}],"groups":[{"type":"docs","tasks":[{"task":{"id":"d00001","type":"docs","title":"README","status":"todo","file":".sya/tasks/d00001-readme.md"},"icon":"·"}]}],"archived_count":3,"markdown":"# sya\n\n## M2 (2/5 closed) [████░░░░░░]\nCLI milestone\n\n- ⛔ a3f8c1 [impl] Streaming @codex — blocked: blocking relation \"depends_on\" has non-terminal targets\n\nArchived tasks: 3"}}
```

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

### close_ambiguous

Exit code: 3. `close` uses this when `--to` was omitted, the first terminal status is not directly reachable from the current status, and another terminal is reachable. `reachable` lists the terminal transitions that can be selected explicitly, and `hints` contains literal commands.

```json
{"ok":false,"error":{"type":"close_ambiguous","message":"cannot infer close target for feature from impl: use --to","task":"a3f8c1","task_type":"feature","from":"impl","reachable":[{"to":"scrapped","kind":"setback","description":"Task was cancelled with rationale in Log"}],"hints":["sya close a3f8c1 --to scrapped"]}}
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

### wisp_link_forbidden

Exit code: 2. Wisps are not task relation endpoints; squash them into real tasks first.

```json
{"ok":false,"error":{"type":"wisp_link_forbidden","message":"wisps cannot be linked as task relations","id":"w-a3f8c1","hint":"sya wisp squash w-a3f8c1 --type T first"}}
```

## Events JSONL

`.sya/events.jsonl` records transition attempts and denials. `claim` denials may use pseudo-target `working`; `close` denials may use pseudo-target `terminal`. These are not schema statuses; they mean the command could not select a concrete working or terminal target.

`unlink` is idempotent. When the relation edge is absent, JSON success uses `action:"noop"`; when removed, it uses `action:"unlinked"`.
