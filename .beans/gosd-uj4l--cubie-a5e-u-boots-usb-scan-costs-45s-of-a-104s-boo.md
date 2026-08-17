---
# gosd-uj4l
title: 'cubie-a5e: U-Boot''s USB scan costs ~4.5s of a 10.4s boot'
status: in-progress
type: task
created_at: 2026-08-16T19:35:26Z
updated_at: 2026-08-16T23:00:00Z
parent: gosd-h1wv
---

Measured during first hardware bring-up (bean gosd-6pfn). The cubie-a5e boots
in **10.38s** from SPL banner to app running (n=5, spread 0.15s), split:

| phase | time |
|---|---|
| SPL banner → `Starting kernel` (U-Boot) | 9.05s |
| kernel → gosd-init's first line | ~1.25s |
| gosd-init → /app running | 0.06s |

U-Boot is 87% of the boot, and within it the USB stack is the single biggest
item — `starting USB...` to the last `Bus usb@...: 1 USB Device(s) found` takes
**~4.5s**, scanning four controllers to conclude `0 Storage Device(s) found`:

```
[132.519] starting USB...
[137.063] Bus usb@4101000: 1 USB Device(s) found
...
[137.085]        scanning usb for storage devices... 0 Storage Device(s) found
```

We boot from mmc via extlinux and never from USB, so this scan buys nothing on
a gosd image. Dropping it (a defconfig fragment change, e.g. removing USB
storage/`usb_boot` from the boot targets, or `CONFIG_USB_STORAGE` off in
U-Boot only) should cut ~40% off the board's boot time.

Not done as part of bring-up because it changes a shipped artifact, so it needs
the artifacts release dance (see docs/artifacts.md) and should be measured
rather than assumed — and because the board has a blocking issue first
(gosd-84b8).

Worth checking whether the same scan is costing the other extlinux boards
(rock-4se, nanopi-zero2, radxa-zero-3e) the same 4.5s.

## Todos

- [x] Confirm which U-Boot config controls the scan at our pin
- [x] Rebuild, re-measure, and record the new baseline (needs the bench —
      no container runtime or bench access at implementation time; the
      ~4.5s saving below is a hypothesis, not a measurement)
- [ ] Check the other U-Boot boards for the same cost

## Summary of Changes

Root-caused and fixed at the pinned `UBOOT_TAG` (v2026.04) by reading the
actual upstream Kconfig sources rather than guessing, and confirmed against
the real Kconfig engine (cloned u-boot at the pinned tag, ran
`make radxa-cubie-a5e_defconfig` then
`scripts/kconfig/merge_config.sh -m .config <fragment>` +
`make olddefconfig` with host-only tools — no cross-compiler or Docker
needed to validate a `.config` resolution):

- `arch/arm/Kconfig`'s `ARCH_SUNXI` hard-`select`s `CONFIG_CMD_USB`,
  `CONFIG_USB_STORAGE` and `CONFIG_USB_KEYBOARD` whenever
  `DISTRO_DEFAULTS && USB_HOST` — both true for this board, and neither
  changeable from a fragment (`select` always wins over a "not set" in a
  merged fragment) without also removing real USB host/storage support or
  the distro-boot mechanism the mmc/extlinux path needs. Left untouched.
- The actual trigger is `boot/Kconfig`'s `CONFIG_PREBOOT`, which — because
  `CONFIG_USB_KEYBOARD` is on — defaults to `"usb start"`, run
  unconditionally by `common/main.c`'s `main_loop()` before the boot-delay
  countdown, purely so a USB keyboard could interrupt autoboot. Unlike the
  selects above, this is a plain `default`, which a fragment CAN override.
- New fragment `build/boards/cubie-a5e/uboot/skip-usb-scan.config` sets
  `CONFIG_PREBOOT=""`. Verified by regenerating the real `.config`: it
  sticks, `CONFIG_PREBOOT_DEFINED` drops out with it, and
  `CONFIG_CMD_USB`/`CONFIG_USB_STORAGE`/`CONFIG_USB_HOST`/`CONFIG_DM_USB`/
  `CONFIG_USB_GADGET`/`CONFIG_BOOTSTD`/`CONFIG_DISTRO_DEFAULTS`/
  `CONFIG_BOOTCOMMAND` are all byte-for-byte unchanged — mmc/extlinux boot
  and USB gadget (MUSB peripheral mode, `--usb-gadget`) are both untouched.
- Wired into `build/boards/cubie-a5e/uboot/Dockerfile` (three-way
  `merge_config.sh -m`), `README.md` (new "Boot-time" section + Known-gaps
  note), `.github/workflows/build-artifacts.yml`'s provenance `--arg
  config` string, and `.changeset/cubie-a5e-skip-usb-scan.md`
  (`artifacts: patch`).
- `internal/artifacts.Version` deliberately NOT bumped (tag-first,
  bump-second — this only reaches real builds after an `artifacts`
  release).
- NOT done: the actual rebuild + bench re-measurement (no container
  runtime / bench access available), and checking whether rock-4se,
  nanopi-zero2 and radxa-zero-3e pay the same cost — both left as open
  todos above.


## Verification of the config change (2026-08-16, no bench)

The root cause was confirmed against the real Kconfig engine, not by reading
sources alone: generating `radxa-cubie-a5e_defconfig` at the pinned
`UBOOT_TAG` inside the board's own build container and merging the fragments
shows the before/after exactly.

Baseline (defconfig + the two existing fragments):

```
CONFIG_PREBOOT="usb start"
CONFIG_PREBOOT_DEFINED=y
```

With `skip-usb-scan.config`: `CONFIG_PREBOOT=""` and `CONFIG_PREBOOT_DEFINED`
drops out entirely, so `main_loop()`'s `env_get("preboot")` returns NULL and
the block never runs.

Everything that must survive does, verified in the same run:
`CONFIG_CMD_USB=y`, `CONFIG_USB_STORAGE=y`, `CONFIG_USB_HOST=y`,
`CONFIG_DM_USB=y`, **`CONFIG_USB_GADGET=y`** (so `--usb-gadget` is
untouched), `CONFIG_BOOTSTD=y`, `CONFIG_DISTRO_DEFAULTS=y`,
`CONFIG_BOOTCOMMAND="run distro_bootcmd"`, `CONFIG_BOOTDELAY=0` and the
`dram-1gb.config` values.

**Now measured** — see the bench results below.


## Bench result — measured 2026-08-17

Rebuilt U-Boot from this recipe (the fragment merged by the board's own
Dockerfile) and booted it on the 1GB board.

**The scan is gone.** No `starting USB...`, no `Bus usb@...: N USB Device(s)
found`, no `scanning usb for storage devices` — zero such lines across every
boot captured.

**Timing, 5 clean power cycles** (the baseline's own methodology):

| phase | before (gosd-6pfn) | after | delta |
|---|---|---|---|
| U-Boot (SPL banner → `Starting kernel`) | 9.05s | **4.50s** (spread 0.03s) | **−4.55s** |
| total (SPL banner → app listening) | 10.38s (spread 0.15s) | 6.98s mean (6.70–7.75) | ≈ −3.4s |

The U-Boot phase is the clean apples-to-apples figure and it is very tight
(0.03s across five boots): −4.55s, which recovers essentially all of the
4.5s the scan was measured to cost. The end-to-end totals improve by ~3.4s
but vary more than the baseline's did (spread 1.04s vs 0.15s), because the
variance now sits after the kernel starts and this run used
`examples/hello` rather than the bring-up session's image — so the total is
reported as a range rather than a new precise baseline.

Boot is otherwise normal: mmc/extlinux discovery, kernel, gosd-init, `/app`,
and on these boots DHCP, mDNS and NTP all came up
(`eth0: lease {192.168.1.201 ...}`, `mdns: answering as hello.local`).

`CONFIG_USB_GADGET` remains set, so `--usb-gadget` is unaffected — untested
here only because this board's USB-C carries its bench power (bean
gosd-6pfn).
