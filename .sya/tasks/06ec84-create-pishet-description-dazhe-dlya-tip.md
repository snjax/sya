---
id: 06ec84
type: bug
title: "create пишет ## Description даже для типов без объявленной секции — мгновенный section_unknown в doctor"
status: done
priority: normal
created: "2026-06-12T02:36:37.389952341Z"
schema_version: 1
---
## Description
Найдено при миграции трекера: epic без sections:[Description] в схеме -> sya create пишет Description -> doctor: error section_unknown. Варианты: create уважает sections типа (пишет только объявленные; ничего, если список пуст); или undeclared section = warning, не error. Решить и реализовать.

## Log
- 2026-06-12T02:36:37Z @snjax: created
- 2026-06-12T02:42:54Z @snjax: open -> impl
- 2026-06-12T02:48:29Z @snjax: impl -> verify
- 2026-06-12T02:48:29Z @snjax: verify -> done
