---
id: 80c5c1
type: task
title: sya template apply (PRD 17.2) + graph + roadmap (17.4-17.5)
status: done
priority: normal
parent: 0a6cff
created: "2026-06-12T05:11:51.579385676Z"
schema_version: 1
---
## Description
.sya/templates/*.yml: params, tasks с key-ссылками в relations, {{param}} подстановка, preflight целиком, map key->id, --dry-run, --parent. graph: mermaid/dot граф экземпляров по blocking-связям, parent=подграфы, archived пунктир. roadmap: детерминированный Markdown: эпики с прогрессом и статус-иконками, -o file.
## Log
- 2026-06-12T05:11:51Z @snjax: created
- 2026-06-12T05:11:51Z @snjax: open -> spec
- 2026-06-12T05:11:51Z @snjax: spec -> impl
- 2026-06-12T05:48:15Z @snjax: impl -> unit_test
- 2026-06-12T05:48:15Z @snjax: unit_test -> func_test
- 2026-06-12T05:48:15Z @snjax: func_test -> integ_test
- 2026-06-12T05:48:15Z @snjax: integ_test -> audit
- 2026-06-12T05:48:15Z @snjax: audit -> done
