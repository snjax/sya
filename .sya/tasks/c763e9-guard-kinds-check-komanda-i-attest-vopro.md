---
id: c763e9
type: task
title: "Guard kinds: check (команда) и attest (вопрос с обоснованием)"
status: open
priority: normal
parent: f1a9a8
relations:
  relates:
  - c81ca0
created: "2026-06-12T02:34:28.259243034Z"
schema_version: 1
---
## Description
check: shell-команда из схемы, exit 0 = pass (детерминированные проверки: тесты, cloc-метрики; trust-модель git-хуков). attest: вопрос на переходе, проходит только с --attest id="yes: <обоснование>"; ответ пишется в Log и events.jsonl. Только для немеханизируемых суждений.

## Log
- 2026-06-12T02:34:28Z @snjax: created
