---
id: 5579c8
type: bug
title: "claude-плагин: marketplace.json owner-строка + битый frontmatter в commands/ready.md"
status: done
priority: normal
created: "2026-06-12T03:08:18.741333207Z"
schema_version: 1
---
## Description
Репро: claude plugin validate . -> 'owner: expected object, received string' (юзер словил при /plugin add). claude plugin validate ./claude-plugin -> frontmatter ready.md не парсится (молча грузится пустым). Фикс + тесты: структурная валидация манифестов и frontmatter всех команд/скиллов в go-тестах; exec-тест claude plugin validate при наличии бинарника; just validate-plugin.
## Log
- 2026-06-12T03:08:18Z @snjax: created
- 2026-06-12T03:08:18Z @snjax: open -> impl
- 2026-06-12T03:12:22Z @snjax: impl -> verify
- 2026-06-12T03:12:22Z @snjax: verify -> done
