---
# gosd-aur4
title: 'staticelf: GOARM mismatch is undetectable — an armv7 binary passes verification aimed at pi-zero-w''s armv6'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:53:52Z
updated_at: 2026-07-31T07:53:52Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area). Mechanism
verified experimentally by the reviewing agent: GOARM=5/6/7 builds of the
same program all produce identical e_flags (0x5000002); Go's linker does
not encode GOARM anywhere debug/elf's Class/Machine/Flags expose.

`staticelf.Verify` checks Class/Machine and no-PT_INTERP only; `GOARM` is
used purely for error-message text (staticelf.go:41-50). A --with-external
or build-external binary compiled with the Go default (GOARM=7) for
pi-zero-w (real armv6 silicon) passes verification and can hit an
illegal-instruction fault on hardware — the exact class of failure
staticelf exists to prevent.

**Fix options (pick one):** parse .ARM.attributes' Tag_CPU_arch (Go's
linker does emit it and it varies with GOARM) and check it for arm
targets; or document the gap explicitly in the package doc + the externals
docs so it isn't assumed covered. Either way add a test with a
wrong-GOARM fixture binary (or a documented note explaining why not).
