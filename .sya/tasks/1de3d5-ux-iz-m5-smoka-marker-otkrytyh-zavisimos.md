---
id: 1de3d5
type: bug
title: "UX из M5-смока: маркер открытых зависимостей в ready + видимый успех check/attest"
status: done
priority: normal
created: "2026-06-12T05:42:22.663674243Z"
schema_version: 1
---
## Description
Смок: (1) ready показывает draft-фичи с открытыми depends_on — это по спеке (blocking гейтит только working/terminal), но агент дезориентирован: добавить информационный маркер deps_open (human: '[deps: N open]', json: pending_deps); (2) при успешном move с check/attest guard'ами успех тихий — печатать '✓ check ...: passed' / '✓ attested <id>' в stderr.
## Log
- 2026-06-12T05:42:22Z @snjax: created
- 2026-06-12T05:42:22Z @snjax: open -> impl
- 2026-06-12T05:48:44Z @snjax: impl -> verify
- 2026-06-12T05:48:44Z @snjax: verify -> done
