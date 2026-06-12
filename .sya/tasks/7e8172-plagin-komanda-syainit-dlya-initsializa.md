---
id: 7e8172
type: task
title: "плагин: команда /sya:init для инициализации sya в проекте"
status: done
priority: normal
created: "2026-06-12T03:18:04.883153154Z"
schema_version: 1
---
## Description
Аналог beads:init. claude-plugin/commands/init.md: проверить бинарник (если нет — curl|sh инсталлер из README), sya init [--prefix из имени каталога или аргумента], показать созданное, предложить: sya-board для кастомного workflow, AGENTS.md-сниппет для других агентов, первый таск, коммит .sya. Бамп версии плагина 0.1.0->0.1.1 в plugin.json и marketplace.json. Frontmatter обязан проходить claude plugin validate (тест уже есть).
## Log
- 2026-06-12T03:18:04Z @snjax: created
- 2026-06-12T03:18:04Z @snjax: open -> spec
- 2026-06-12T03:18:05Z @snjax: spec -> impl
- 2026-06-12T03:32:12Z @snjax: impl -> unit_test
- 2026-06-12T03:32:12Z @snjax: suite 12 пакетов зелёный
- 2026-06-12T03:32:12Z @snjax: unit_test -> func_test
- 2026-06-12T03:32:12Z @snjax: claude plugin validate OK; live-эксперименты: symlink/hardlink целы, init идемпотентен
- 2026-06-12T03:32:12Z @snjax: func_test -> integ_test
- 2026-06-12T03:32:12Z @snjax: integ_test -> audit
- 2026-06-12T03:32:12Z @snjax: audit: in-place запись только для agent-доков с комментарием-обоснованием, маркерный блок, SameFile-детект — CLEAN
- 2026-06-12T03:32:12Z @snjax: audit -> done
