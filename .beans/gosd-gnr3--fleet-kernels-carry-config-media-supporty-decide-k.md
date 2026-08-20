---
# gosd-gnr3
title: Fleet kernels carry CONFIG_MEDIA_SUPPORT=y — decide keep or trim
status: todo
type: task
priority: normal
created_at: 2026-08-08T03:45:52Z
updated_at: 2026-08-20T04:58:09Z
---

Split out of [[gosd-10fn]] at its close-out (btrfs shipped in
artifacts/v0.10.0; this question remained). Every board's kernel — fleet AND
Pi — builds the media subsystem in via defconfig inheritance. Nothing in GoSD
uses it.

## DECISION (JP, 2026-08-20): trim, riding a natural release

Trim `CONFIG_MEDIA_SUPPORT` fleet-wide, in the same shape as the btrfs pass
that split this bean out: fragment lines plus a `ForbiddenY` assertion. Do
**not** cut an artifacts release for this alone — it has no user-visible
payoff. Land it on whichever artifacts release comes next for another reason.

Re-adding it later, if a camera or capture feature ever lands, is a single
fragment line. That asymmetry is why trimming now is cheap and keeping is not.

## Locked decisions

- **Verify against the published artifact, never the committed snapshot.**
  `build/boards/*/kernel.config` is a stale snapshot — it is only rewritten by
  an actual `gosd build-kernel` run, so it lags each board's `kernel.fragment`
  by however many releases. Establish that `CONFIG_MEDIA_SUPPORT=y` is really
  in the shipped kernel per board via
  `gh release download artifacts/vX.Y.Z -p '<board>.tar.zst'`, which carries
  the real `kernel.config`. **This is not pedantry:** [[gosd-95yu]] read "the
  Pi family has no ext4" off these snapshots and that single wrong fact
  reached a build-time refusal, a user-facing runtime error and a published
  release note before [[gosd-ssth]] undid it. Do not repeat it here.
- **The fragment is the assertion**, so the trim is a fragment line per board,
  plus a `ForbiddenY` entry so it cannot silently return via a future
  defconfig inheritance. Mirror whatever [[gosd-10fn]] did for btrfs rather
  than inventing a second mechanism.
- **Kernel pins are per-family and bump family-wide** — the mainline fleet
  (Rockchip, Allwinner, qemu-virt) shares `fleetKernelTag`; the Pis share the
  `piZeroCommitRef` downstream commit pin. This trim touches every board, so
  it is a fleet-wide fragment change, not a per-board one.
- **Tag-first, bump-second.** Ship the fragment change WITHOUT bumping
  `internal/artifacts.Version`, with an `artifacts:` change file. Bumping to
  an unpublished tag turns the qemu boot-to-HTTP CI job red. A separate
  follow-up PR bumps `Version` after the release exists and verifies it three
  ways (clean-machine build, offline re-run, content spot-check).

## Risk to check before trimming

`MEDIA_SUPPORT` is `select`ed by more than camera drivers. Before assuming
it is dead weight, confirm nothing GoSD ships depends on it transitively —
in particular anything in the DRM/HDMI path (CEC lives under the media tree
on some SoCs) and anything the `sound` work pulled in. If a board turns out
to need it, trim the boards that do not and record which board kept it and
why, rather than abandoning the whole pass.

## Todo

- [x] JP decision: trim, on a natural release
- [ ] Confirm `CONFIG_MEDIA_SUPPORT=y` in the PUBLISHED artifact for each board
- [ ] Confirm nothing shipped depends on it (DRM/HDMI/CEC, sound)
- [ ] Fragment line per board + `ForbiddenY` assertion, mirroring the btrfs pass
- [ ] `artifacts:` change file; do NOT bump `internal/artifacts.Version`
- [ ] Confirm the kernel actually shrank (record the before/after size)
- [ ] Follow-up PR after the release: bump `Version`, verify three ways
