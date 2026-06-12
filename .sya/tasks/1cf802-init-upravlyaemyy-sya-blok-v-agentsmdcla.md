---
id: 1cf802
type: task
title: "init: управляемый sya-блок в AGENTS.md/CLAUDE.md, link-безопасная запись"
status: done
priority: normal
created: "2026-06-12T03:27:36.440584141Z"
schema_version: 1
---
## Description
Best practice beads: bd init пишет AGENTS.md идемпотентно (маркер BEGIN ... INTEGRATION), in-place запись сохраняет sym/hard-линки. sya init должен: создавать/дополнять AGENTS.md минимальным маркированным блоком (lean pointer: prime + schema docs, ~10 строк); CLAUDE.md — только если уже существует: если SameFile с AGENTS.md (хардлинк или симлинк) — пропустить (один файл!), иначе тот же блок; запись СТРОГО in-place (не temp+rename — rename разрывает линк); idempotent re-run обновляет блок между маркерами; --no-agents-md для отключения. Тесты: create/append/re-run/marker-update + симлинк в обе стороны + хардлинк-пара (SameFile, линк цел после записи) + раздельные файлы.
## Log
- 2026-06-12T03:27:36Z @snjax: created
- 2026-06-12T03:27:36Z @snjax: open -> spec
- 2026-06-12T03:27:36Z @snjax: spec -> impl
- 2026-06-12T03:32:12Z @snjax: impl -> unit_test
- 2026-06-12T03:32:12Z @snjax: suite 12 пакетов зелёный
- 2026-06-12T03:32:12Z @snjax: unit_test -> func_test
- 2026-06-12T03:32:12Z @snjax: claude plugin validate OK; live-эксперименты: symlink/hardlink целы, init идемпотентен
- 2026-06-12T03:32:12Z @snjax: func_test -> integ_test
- 2026-06-12T03:32:12Z @snjax: integ_test -> audit
- 2026-06-12T03:32:12Z @snjax: audit: in-place запись только для agent-доков с комментарием-обоснованием, маркерный блок, SameFile-детект — CLEAN
- 2026-06-12T03:32:12Z @snjax: audit -> done
