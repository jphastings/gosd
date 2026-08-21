---
# gosd-ylkv
title: 'U-Boot preboot USB scan: measure it on rock-4se, nanopi-zero2 and radxa-zero-3e'
status: todo
type: task
created_at: 2026-08-21T06:53:00Z
updated_at: 2026-08-21T06:53:00Z
---

Split out of bean `gosd-uj4l` (2026-08-21), which fixed this on cubie-a5e and
left "check the other U-Boot boards for the same cost" as separate scope that
merely happened to be noticed there. This bean owns that survey for
**rock-4se**, **nanopi-zero2** and **radxa-zero-3e**.

The question: does U-Boot run an unconditional USB scan before boot on these
boards too, and if so, what does it actually cost? gosd images boot from
mmc via extlinux and never from USB, so any such scan buys nothing on a gosd
image — but "buys nothing" is not the same as "costs something worth fixing",
which is why this starts with a measurement.

## Two things to know before starting

**1. cubie-a5e's saving is measured; nothing here is.** The original bean's
~4.5s figure WAS a hypothesis at implementation time — reasoned from the
upstream Kconfig sources with no bench access, as its own todo list records —
but it has since been confirmed on hardware (2026-08-17, artifacts v0.10.2's
release note): the board's U-Boot phase fell from 9.05s to 4.50s across 5
clean power cycles, spread 0.03s. Do NOT carry that number across to these
three boards. They are a different SoC family with a different defconfig, no
recorded U-Boot-phase baseline at all, and possibly no scan. Measuring each
board — a serial-console boot capture, timestamped, several clean power
cycles, before and after — is this bean's first job and the thing that decides
whether there is any fix to make.

**2. The cubie-a5e root cause does not transfer.** There, the trigger was
`arch/arm/Kconfig`'s `config ARCH_SUNXI` hard-`select`ing `CONFIG_USB_KEYBOARD`
whenever `DISTRO_DEFAULTS && USB_HOST`, which in turn made `boot/Kconfig`'s
`config PREBOOT` default to `"usb start"` — run unconditionally by
`common/main.c`'s `main_loop()` before the boot-delay countdown. The fix was
one line, `CONFIG_PREBOOT=""`, because a Kconfig `default` (unlike a `select`)
is overridable from a fragment. These three boards are **Rockchip**
(`ARCH_ROCKCHIP`), so that specific `select` chain is not theirs. Establish
each board's own `CONFIG_PREBOOT` value at the pinned `UBOOT_TAG` before
assuming anything — the same way `gosd-uj4l` did, by generating the board's
defconfig with the real Kconfig engine (`make <board>_defconfig` then
`scripts/kconfig/merge_config.sh -m` + `make olddefconfig`, host tools only,
no cross-compiler or Docker needed to validate a `.config` resolution).

## Todos

- [ ] Baseline: capture and timestamp a serial boot for each of rock-4se,
      nanopi-zero2 and radxa-zero-3e (SPL/TPL banner → `Starting kernel` →
      gosd-init → /app), several clean power cycles each, and record the
      U-Boot-phase figure — none of these boards has one on record
- [ ] Establish whether a USB scan happens at all on each: look for
      `starting USB...` / `Bus usb@...: N USB Device(s) found` /
      `scanning usb for storage devices` in those captures, and read each
      board's resolved `CONFIG_PREBOOT` at the pinned `UBOOT_TAG`
- [ ] For each board where a scan happens AND costs enough to be worth it:
      add a `skip-usb-scan.config` fragment mirroring cubie-a5e's, wired
      into that board's `uboot/Dockerfile` three-way merge and its
      `build-artifacts.yml` provenance config string
- [ ] Prove nothing else moved: `CONFIG_CMD_USB`, `CONFIG_USB_STORAGE`,
      `CONFIG_USB_HOST`, `CONFIG_DM_USB`, `CONFIG_USB_GADGET`,
      `CONFIG_BOOTSTD`, `CONFIG_DISTRO_DEFAULTS` and `CONFIG_BOOTCOMMAND`
      byte-for-byte unchanged in the regenerated `.config` (mmc/extlinux
      boot and `--usb-gadget` must be untouched)
- [ ] Re-measure on hardware after the change, same methodology as the
      baseline, and record before/after here
- [ ] Record the negative results too: a board where no scan happens, or
      where it costs little enough not to bother, is a finding worth writing
      down so nobody re-opens this

## Release mechanics — tag-first, bump-second

Any fix here is a U-Boot defconfig change under `build/boards/*`, so it only
reaches real (non-`--artifacts-dir`) builds after a new `artifacts/vX.Y.Z`
GitHub release. Ship the fragment WITHOUT touching `internal/artifacts.Version`,
with an `artifacts:` change file; the version pin moves in a separate follow-up
PR once the release exists. Bumping the pin in the same PR points it at an
unpublished tag and turns the qemu boot-to-HTTP CI job red. The full procedure
is in the artifacts documentation.

No parent: this spans three boards and belongs to none of their epics.
