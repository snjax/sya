---
id: 0e71f1
type: task
title: "curl|sh инсталлер: только golang в зависимостях, linux+macos"
status: func_test
priority: normal
created: "2026-06-12T02:40:16.29384792Z"
schema_version: 1
---
## Description
Спека: scripts/install.sh — POSIX sh (без bashизмов), работает через `curl -fsSL <raw-url> | sh` под любым шеллом на linux и macos.
Поведение:
1. Проверить наличие go (>=1.26, т.к. go.mod требует 1.26.3; при старее — понятная ошибка с ссылкой на go.dev/dl; toolchain-автодокачку Go использовать как fallback не запрещаем).
2. `go install github.com/snjax/sya/cmd/sya@latest` (override: env SYA_VERSION=<module-version>).
3. Определить GOBIN: `go env GOBIN`, иначе `$(go env GOPATH)/bin`.
4. Скопировать бинарник в SYA_INSTALL_DIR (default ~/.local/bin), mkdir -p.
5. Если install dir не в PATH — напечатать per-shell подсказку (bash/zsh: export в rc-файл; fish: fish_add_path).
6. Финал: вывести установленную версию (`<dir>/sya version`).
Никаких sudo, git, just — только go. set -eu, аккуратные сообщения об ошибках в stderr, exit 1.
README: секция Install с curl|sh однострочником + альтернатива go install.
Приёмка: shellcheck чистый; docker-матрица bash/zsh/fish на golang:1.26 — установка с raw URL и `sya version` работают.
## Log
- 2026-06-12T02:40:16Z @snjax: created
- 2026-06-12T02:40:16Z @snjax: open -> spec
- 2026-06-12T02:40:16Z @snjax: spec -> impl
- 2026-06-12T02:48:29Z @snjax: impl -> unit_test
- 2026-06-12T02:48:29Z @snjax: unit-ярус: go-тесты не затронуты скриптом, suite 10/10 зелёный; sh -n чистый
- 2026-06-12T02:48:29Z @snjax: unit_test -> func_test
