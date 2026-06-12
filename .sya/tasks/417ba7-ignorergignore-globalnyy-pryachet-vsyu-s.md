---
id: 417ba7
type: bug
title: ".ignore/.rgignore: глобальный '*' прячет всю .sya — должен прятать только базу issues"
status: done
priority: normal
created: "2026-06-12T03:20:36.759606958Z"
schema_version: 1
---
## Description
Сейчас init пишет '*': скрыто всё, включая schema.yml/config.yml — а борды должны быть observable для AI. Правильный scope: tasks/, wisps/, events.jsonl, .lock. Наблюдаемыми остаются schema.yml, config.yml, memory/. Исправить: cmd init (контент файлов), doctor (--fix и проверка контента, миграционный hint для старых проектов с '*'), тесты+goldens, AGENTS.md и claude-plugin тексты (init.md, SKILL.md), .sya этого репо.
## Log
- 2026-06-12T03:20:36Z @snjax: created
- 2026-06-12T03:20:36Z @snjax: open -> impl
- 2026-06-12T03:32:12Z @snjax: impl -> verify
- 2026-06-12T03:32:12Z @snjax: verify -> done
