---
id: 0e71f1
type: task
title: "curl|sh инсталлер: только golang в зависимостях, linux+macos"
status: done
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
- 2026-06-12T03:02:24Z @snjax: func_test -> integ_test
- 2026-06-12T03:02:24Z @snjax: func+integ ярусы: docker-матрица golang:1.26 — bash/zsh/fish, установка с raw URL, sya version (псевдоверсия модуля), fish_add_path хинт при SHELL=fish. GOPROXY=direct для обхода кэша
- 2026-06-12T03:02:24Z @snjax: integ_test -> audit
- 2026-06-12T03:02:24Z @snjax: audit: sh -n чистый; POSIX-only конструкции; set -eu; ошибки в stderr; нет sudo/git/just зависимостей — AUDIT CLEAN
- 2026-06-12T03:02:24Z @snjax: audit -> done
