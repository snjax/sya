---
id: c31744
type: bug
title: "install.sh: PATH-подсказка не учитывает fish"
status: scrapped
priority: normal
created: "2026-06-12T02:53:43.118266874Z"
schema_version: 1
---
## Description
fish не читает ~/.profile; при $SHELL=*fish печатать fish_add_path <dir>. zsh -> ~/.zshrc, bash -> ~/.bashrc или ~/.profile.

## Log
- 2026-06-12T02:53:43Z @snjax: created
- 2026-06-12T02:53:43Z @snjax: open -> impl
- 2026-06-12T02:56:07Z @snjax: impl -> scrapped ↩: не баг скрипта: per-shell детект через $SHELL уже был; докер-тест не выставлял SHELL. Codex добавил мелкое улучшение хинтов — принято
