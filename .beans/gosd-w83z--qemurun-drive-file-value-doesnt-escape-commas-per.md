---
# gosd-w83z
title: 'qemurun: -drive file= value doesn''t escape commas per QEMU option syntax'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-07-31T07:54:53Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

qemurun.go:195 builds `if=none,file=<path>,format=raw,id=hd0` by
concatenation; QEMU requires literal commas in values to be doubled.
gosd-generated paths never contain commas, but `gosd qemuboot <img>` takes
a user path — a comma-bearing path misparses (error, or wrong file
attached).

**Fix:** double literal commas in imgPath before building the -drive
value.
