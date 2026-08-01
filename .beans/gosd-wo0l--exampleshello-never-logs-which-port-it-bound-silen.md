---
# gosd-wo0l
title: 'examples/hello: never logs which port it bound — silent :80→:8080 fallback confuses serial-console debugging'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-07-31T07:54:53Z
---

Found by review sweep `gosd-fuxs` (cross-cutting area), verified.

examples/hello/main.go:44-48 falls back from :80 to :8080 with no log
line; the startup banner prints before Listen. Someone watching the
serial console has no way to know the app moved ports (docs and examples
assume :80).

**Fix:** log listener.Addr() after Listen succeeds.
