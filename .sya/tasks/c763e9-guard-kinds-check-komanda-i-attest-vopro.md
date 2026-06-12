---
id: c763e9
type: task
title: "Guard kinds: check (команда) и attest (вопрос с обоснованием)"
status: done
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
- 2026-06-12T05:49:01Z @snjax: open -> spec
- 2026-06-12T05:49:01Z @snjax: spec -> impl
- 2026-06-12T05:49:01Z @snjax: impl -> unit_test
- 2026-06-12T05:49:01Z @snjax: unit_test -> func_test
- 2026-06-12T05:49:01Z @snjax: func_test -> integ_test
- 2026-06-12T05:49:01Z @snjax: integ_test -> audit
- 2026-06-12T05:49:01Z @snjax: audit -> done
