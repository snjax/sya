---
id: a767ab
type: task
title: Guard kinds check+attest (PRD 17.3) + duplicates (17.4)
status: done
priority: normal
parent: 0a6cff
relations:
  relates:
  - c763e9
created: "2026-06-12T05:11:51.543976796Z"
schema_version: 1
---
## Description
check: sh -c из корня, env SYA_TASK_ID/FILE, timeout, exit0=pass, CheckRunner-интерфейс инжектится из cli. attest: --attest id="yes: обоснование" на move/close/claim, запись в Log+events, violation с вопросом и синтаксисом флага. Оба deferred вне move-времени (ready/transitions помечают, не исполняют). duplicates: токены+триграммы cosine, пары+score+hint, без LLM.
## Log
- 2026-06-12T05:11:51Z @snjax: created
- 2026-06-12T05:11:51Z @snjax: open -> spec
- 2026-06-12T05:11:51Z @snjax: spec -> impl
- 2026-06-12T05:24:38Z @snjax: Implemented check/attest guards, duplicates command, textsim package, and tests; just test && just lint pass.
- 2026-06-12T05:48:15Z @snjax: impl -> unit_test
- 2026-06-12T05:48:15Z @snjax: unit_test -> func_test
- 2026-06-12T05:48:15Z @snjax: func_test -> integ_test
- 2026-06-12T05:48:15Z @snjax: integ_test -> audit
- 2026-06-12T05:48:15Z @snjax: audit -> done
