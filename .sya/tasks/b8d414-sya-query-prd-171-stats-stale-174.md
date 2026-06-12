---
id: b8d414
type: task
title: sya query (PRD 17.1) + stats + stale (17.4)
status: done
priority: normal
parent: 0a6cff
created: "2026-06-12T05:11:51.561804848Z"
schema_version: 1
---
## Description
Язык запросов: рекурсивный спуск, ключи id/type/status/priority/title/assignee/parent/label/field.*/rel.*/age + булевы ready/blocked/archived/terminal/working/parked/dead_end; операторы = != ~ > >= < <= in; and/or/not/скобки; FuzzQuery. stats: счётчики тип×статус, ready/blocked/deadend/карантин/возраст. stale --days N: последняя Log-запись старше N дней.
## Log
- 2026-06-12T05:11:51Z @snjax: created
- 2026-06-12T05:11:51Z @snjax: open -> spec
- 2026-06-12T05:11:51Z @snjax: spec -> impl
- 2026-06-12T05:48:15Z @snjax: impl -> unit_test
- 2026-06-12T05:48:15Z @snjax: unit_test -> func_test
- 2026-06-12T05:48:15Z @snjax: func_test -> integ_test
- 2026-06-12T05:48:15Z @snjax: integ_test -> audit
- 2026-06-12T05:48:15Z @snjax: audit -> done
