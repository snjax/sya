---
id: c24ed5
type: bug
title: "version: 'dev' при go install @latest — нужен fallback на debug.ReadBuildInfo"
status: done
priority: normal
created: "2026-06-12T02:53:43.099351619Z"
schema_version: 1
---
## Description
Docker-матрица: установка через go install даёт sya dev. main.version из ldflags пуст -> читать module version/pseudo-version из runtime/debug.ReadBuildInfo.

## Log
- 2026-06-12T02:53:43Z @snjax: created
- 2026-06-12T02:53:43Z @snjax: open -> impl
- 2026-06-12T02:56:07Z @snjax: impl -> verify
- 2026-06-12T02:56:07Z @snjax: verify -> done
