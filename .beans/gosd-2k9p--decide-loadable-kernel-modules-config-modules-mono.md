---
# gosd-2k9p
title: 'Decide: loadable kernel modules (CONFIG_MODULES) — monolithic forever vs BYO .ko'
status: completed
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-08-20T04:58:00Z
---

Standalone decision, deliberately **not** a child of the [[gosd-47rm]] epic:
that epic covers compiling drivers **in** (`=y`); this bean decides whether
GoSD ever supports **loadable** kernel modules (`.ko`).

## DECISION (JP, 2026-08-20): monolithic forever

`CONFIG_MODULES=n` stays GoSD's locked shape. This is a **closed question**,
not a deferral — do not reopen it in review or design around a future `.ko`
loader. It reopens only if a concrete need surfaces that a compiled-in driver
cannot meet (see "What would reopen this" below).

### Why

The only capability loadable modules add over the status quo is *loading a
driver without rebuilding the kernel*. `gosd build-kernel` ([[gosd-47rm]])
already gives a developer any driver compiled in via one fragment line, with
identical driver coverage, and those rebuilds are content-addressed, cached
and container-local — so the capability being bought is convenience on a path
that is already fast on the second run.

Against that, "yes" would have cost, permanently:

- **A fleet-wide kernel change.** All boards pin per-family tags and bump
  together, so `CONFIG_MODULES=y` is every board plus a full artifacts
  release (tag-first, bump-second).
- **A kernel-devel artifact per board, per arch, every artifacts release,
  forever.** `.ko`s are not portable — vermagic (version + arch + key config)
  and MODVERSIONS CRCs mean a module must be built against GoSD's *exact*
  kernel. Nobody could drop in a Debian or RPiOS `.ko`, so shipping headers,
  `.config`, `Module.symvers` and the release string would become a standing
  release obligation.
- **A signing decision that widens the appliance trust boundary.** An
  unsigned-module door is a real expansion of it, and gosd-init deliberately
  has no interactive surface at all — no shell, no SSH, no remote debug.
  Adding a way to load arbitrary kernel code cuts directly against that.
- **C/Kbuild module tooling**, inherently container-based.

The runtime half was never the problem — `finit_module(2)` via
`golang.org/x/sys/unix` is pure Go and fits the `platform_linux.go` seam
cheaply. The cost is entirely in the artifacts and trust obligations, and
those are the parts that never go away.

### What would reopen this

A concrete case that compiled-in drivers genuinely cannot serve:

- proprietary out-of-tree drivers that cannot ship in a fragment, or
- third-party app ecosystems where **end users** — not the developer who ran
  `gosd build` — attach hardware the image was never compiled for.

If either arrives, reopen with that case named. Absent one, the answer is no.

### Scope note

This decision is about `.ko` loading only. It does not constrain
[[gosd-47rm]], which continues to be the supported way to get a driver GoSD's
stock trimmed kernels cut.

## Original analysis (scoped 2026-07-10, retained)

- **Fleet-wide kernel change**: all boards pin one kernel tag and bump
  together, so `CONFIG_MODULES=y` is every board + a full artifacts release
  (tag-first dance). Decide `MODULE_UNLOAD`, `MODVERSIONS`, and signing
  (`CONFIG_MODULE_SIG`).
- **`.ko`s are not portable**: vermagic and MODVERSIONS CRCs mean a module
  must be built against GoSD's *exact* kernel. So GoSD must publish a
  **kernel-devel artifact** (headers, `.config`, `Module.symvers`,
  release string) per board/arch, every artifacts release, forever.
- **Module build tooling**: compiling a `.ko` is C/Kbuild — a natural
  `build-kernel` extension (the reserved `[[module]]` table in
  `gosd-kernel.toml`).
- **Runtime loader in gosd-init**: `finit_module(2)` via
  `golang.org/x/sys/unix`, pure Go, `platform_linux.go` seam. Load order from
  config (no depmod in v1). Coldplug-at-boot is the sane v1; hotplug would
  additionally need a uevent listener.

## Todo

- [x] Scope what "yes" would require
- [x] JP decision
- [x] Record the rationale and the reopening criteria

## Summary of Changes

Decided: monolithic forever. No code change — the deliverable is the locked
decision and, importantly, the criteria that would reopen it, so a future
agent hitting a driver gap knows whether it qualifies rather than
relitigating the whole question.

Recorded as a project-wide locked decision in CLAUDE.md so it is visible
without reading this bean. The reserved `[[module]]` table in
`gosd-kernel.toml` stays reserved and unused; it is not evidence of a planned
feature.
