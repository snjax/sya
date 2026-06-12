---
id: af4bad
type: bug
title: "close: тихий fallback в scrapped, когда первый terminal недостижим — вопреки PRD"
status: done
priority: normal
created: "2026-06-12T02:56:55.34879902Z"
schema_version: 1
---
## Description
Репро: задача bug в impl; sya close -> 'impl -> scrapped' молча. PRD 7: без --to close обязан ошибиться, если первый достижимый terminal != terminal[0] (т.е. 'успешный' недостижим), перечислив варианты. Агент, вызвавший close на задаче в работе, тихо теряет задачу.

## Log
- 2026-06-12T02:56:55Z @snjax: created
- 2026-06-12T02:56:55Z @snjax: open -> impl
- 2026-06-12T03:02:24Z @snjax: impl -> verify
- 2026-06-12T03:02:24Z @snjax: verify -> done
