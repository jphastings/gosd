---
# gosd-2k9p
title: 'Decide: loadable kernel modules (CONFIG_MODULES) — monolithic forever vs BYO .ko'
status: todo
type: task
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
---

Standalone decision, deliberately **not** a child of the [[gosd-47rm]] epic:
that epic covers compiling drivers **in** (`=y`); this bean decides whether
GoSD ever supports **loadable** kernel modules (`.ko`).

## The question

Stay monolithic forever (`CONFIG_MODULES=n`, today's locked shape), or flip
`CONFIG_MODULES=y` fleet-wide and grow a bring-your-own-`.ko` story?

## What "yes" would require (scoped 2026-07-10)

- **Fleet-wide kernel change**: all boards pin one kernel tag and bump
  together, so `CONFIG_MODULES=y` is every board + a full artifacts release
  (tag-first dance). Decide `MODULE_UNLOAD`, `MODVERSIONS`, and signing
  (`CONFIG_MODULE_SIG` — an unsigned-module door is a real expansion of the
  appliance trust boundary; gosd-init has no interactive surface today).
- **`.ko`s are not portable**: vermagic (version+arch+key config) and
  MODVERSIONS CRCs mean a module must be built against GoSD's *exact* kernel
  — nobody can drop in a Debian/RPiOS `.ko`. So GoSD must publish a
  **kernel-devel artifact** (headers, `.config`, `Module.symvers`,
  release string) per board/arch, every artifacts release, forever.
- **Module build tooling**: compiling a `.ko` is C/Kbuild — inherently
  container-based, a natural `build-kernel` extension (the reserved
  `[[module]]` table in `gosd-kernel.toml`).
- **Runtime loader in gosd-init**: pleasantly cheap — `finit_module(2)` via
  `golang.org/x/sys/unix` (already a dep), pure Go, fits the
  `platform_linux.go` seam. Load order from config (no depmod in v1;
  self-contained modules only). Hotplug-after-boot would additionally need a
  uevent listener; coldplug-at-boot is the sane v1.

## The alternative

The [[gosd-47rm]] epic already gives developers any driver **compiled in** via
a fragment line — same driver coverage, no runtime loader, no kernel-devel
artifacts, no signing question, kernel stays monolithic. The *only* capability
modules add is loading drivers without a kernel rebuild (and rebuilds are
cached, container-local, and per-project).

## Recommendation

Default **no** (stay monolithic) until a concrete need surfaces that
compiled-in drivers can't meet — e.g. proprietary out-of-tree drivers that
can't ship in a fragment, or third-party app ecosystems where end users (not
the developer) attach hardware. Revisit after the epic ships and real
`build-kernel` usage exists.

## Decision needed

JP to choose: monolithic-forever (close this bean, record rationale) or
schedule the modules track (spawn implementation beans per the scope above).
