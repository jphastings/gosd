---
# gosd-aur4
title: 'staticelf: GOARM mismatch is undetectable — an armv7 binary passes verification aimed at pi-zero-w''s armv6'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:53:52Z
updated_at: 2026-08-20T06:18:32Z
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

## Summary of Changes

Investigated the bean's suggested primary fix before implementing it: built real `GOOS=linux GOARCH=arm GOARM=5/6/7` binaries with go1.26 and inspected them with `readelf`/`objdump`. Confirmed the bean's own e_flags finding (identical 0x5000002 across all three GOARM values) AND found the suggested fix itself isn't viable - Go's linker does NOT emit an `.ARM.attributes` section at all for any GOARM value (no such section appears in the ELF at all), so there is no Tag_CPU_arch field to parse. Detecting a GOARM mismatch would need actual instruction-stream disassembly, which is out of scope for an ELF-header verifier.

Took the bean's documented second option instead: added a "Known gap" section to internal/staticelf's package doc and to Verify's doc comment explaining precisely what was checked and why no header-level fix exists, added `TestVerifyCannotDistinguishGOARM` (pins the current, real behavior - Verify accepts a GOARM-mismatched armv6/armv7 binary - so a future Go toolchain change that starts encoding GOARM somewhere would flip this test red and get noticed instead of the gap silently persisting), and added a matching "Known gap" callout to docs/externals.md's fully-static-binary contract section naming pi-zero-w specifically (the fleet's only 32-bit/GOARM board) and telling developers to get GOARM right at the cross-compile step since gosd build-external/--with-external cannot catch a wrong one.
