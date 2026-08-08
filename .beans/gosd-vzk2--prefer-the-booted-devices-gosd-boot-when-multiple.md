---
# gosd-vzk2
title: Prefer the booted device's GOSD-BOOT when multiple GoSD boot partitions exist
status: completed
type: task
priority: normal
created_at: 2026-07-24T07:49:39Z
updated_at: 2026-08-08T19:50:12Z
---

gosd-pcwl fixed gosd-init's boot-partition probe to reject candidates that
merely mount as valid FAT but aren't a GoSD boot partition at all (checks for
gosd.toml at the root; unmounts and keeps probing if absent). That closes the
"vendor image / arbitrary FAT" case, but it cannot distinguish between two
*actual* GoSD boot partitions: an eMMC that itself carries a stale,
previously-flashed GoSD image also has gosd.toml at its root, passes the
sentinel check, and still wins simply by sorting first in device-name order
(mmcblk0 before mmcblk1). The probe has no way to know which physical device
the SoC's boot ROM/U-Boot actually booted from — it only knows device names.

Repro scenario: NanoPi Zero2 (or any board with both eMMC and SD boot media)
with a GoSD image previously flashed to the eMMC, then a *different*/updated
GoSD image freshly flashed to the SD card. gosd-init boots from the SD's
kernel/initramfs (that part is fine — the boot ROM's own media selection
picked the SD), but the boot-partition probe still mounts the eMMC's
GOSD-BOOT as /boot: stale gosd.toml, stale app config, from an image the user
didn't intend to use this boot.

Design direction (not locked, needs its own investigation): have U-Boot pass
the device it actually booted from on the kernel command line (a new
gosd.bootdev-style param, alongside the existing gosd.board/gosd.debug
overrides gosd-init already parses from /proc/cmdline — see
internal/initcfg), and have MountBootPartition prefer/require that device
over walking the candidate list blind. This needs support in every board's
U-Boot config (Rockchip boards' extlinux.conf / boot scripts, Pi boards'
config.txt+start.elf chain), so scope and feasibility per board need
checking before committing to it — may not be uniformly available depending
on what each board's U-Boot/bootloader exposes.

Out of scope for gosd-pcwl (see that bean's "Known residual" note) —
tracked here as a separate, unscoped follow-up.

Cross-reference: [[gosd-pcwl]]

## Investigation findings (2026-07-25)

The design direction above assumed U-Boot could pass the booted device via
`gosd.bootdev=${devtype}${devnum}` in extlinux.conf's APPEND. Verified
against our pinned U-Boot sources (nanopi-zero2: v2026.07-rc5;
radxa-zero-3e, rock-4se: v2026.04):

- **APPEND lines ARE `${var}`-expanded**: `label_boot()` in
  `boot/pxe_utils.c` runs the append string through
  `cli_simple_process_macros()` (line 654 in both pinned tags; also line
  305 for localboot) before setting `bootargs`. Expansion itself is real.
- **But bootstd never sets `devtype`/`devnum`**: all three boards boot via
  standard boot (bootstd) with `BOOTMETH_EXTLINUX` (see each board's
  `bootdelay0.config`), and nothing in that path exports the boot device to
  the environment — verified by inspection of the pinned tags'
  `boot/bootmeth_extlinux.c`, `boot/bootflow.c`, `boot/bootdev-uclass.c`,
  `cmd/bootflow.c`, `arch/arm/mach-rockchip/board.c` and
  `include/configs/rockchip-common.h` (zero `devtype`/`devnum` env_set
  anywhere; the only bootstd env write is `bootargs`). Those variables are
  a distro-bootcmd-era convention; only `bootmeth_script.c` (boot.scr
  compat, which we don't use) sets them under bootstd. `git log` of
  `bootmeth_extlinux.c` shows no compat shim was ever added upstream.
- **Consequence**: `gosd.bootdev=${devtype}${devnum}` in our generated
  extlinux.conf would expand to the empty string on every Rockchip board —
  harmless (gosd-init treats empty as absent) but useless. Emitting it now
  was therefore NOT done; the Rockchip emission needs a U-Boot-side change
  (patch bootmeth_extlinux/pxe to export the device, as bootmeth_script
  does) and hence an artifacts release, plus per-board verification that
  U-Boot's mmc devnum matches Linux's mmcblk index (both follow DT aliases,
  but that must be proven on the bench, not assumed).

Per-board feasibility:

| Board | Boot path | Ambiguity possible? | `gosd.bootdev` emission today |
| --- | --- | --- | --- |
| nanopi-zero2 | bootstd+extlinux (v2026.07-rc5) | YES (eMMC + SD; the repro board) | No — needs U-Boot change ⇒ artifacts release |
| rock-4se | bootstd+extlinux (v2026.04) | YES (eMMC socket + SD) | No — same as nanopi-zero2 |
| radxa-zero-3e | bootstd+extlinux (v2026.04) | No (no eMMC on the 3E variant) | No — and not needed |
| pi-zero-2w / pi-zero-w | start.elf + static cmdline.txt (no expansion) | No (SD-only, no eMMC) | N/A — cannot arise, nothing to emit |
| qemu-virt | our own `qemu -append` | Single virtio disk | YES — emitted (`gosd.bootdev=vda`), CI-exercised |

## Implemented (this PR — no artifacts release needed)

- `internal/initcfg`: `ParseCmdline` now reads `gosd.bootdev=<name>` (a
  kernel block-device name, optional `/dev/` prefix) into
  `CmdlineArgs.BootDev`.
- `cmd/gosd-init/internal/boot`: new pure `FilterBootDevices` narrows the
  GOSD-BOOT candidate list to the named disk's partitions; `Run` applies it
  before `MountBootPartition` and logs the restriction. Restricting rather
  than reordering is deliberate: a reorder would still let a stale eMMC win
  any retry round in which the SD's device node hadn't appeared yet, while
  the booted disk's partition 1 is the very partition the kernel/initramfs
  were loaded from, so probing only it cannot lose an otherwise-good boot.
  Absent/empty/unmatched `gosd.bootdev` keeps the existing full walk (old
  images and all current real-hardware images unaffected).
- `internal/qemurun`: qemu-virt's `-append` now carries `gosd.bootdev=vda`,
  so the CI boot-to-HTTP job exercises the parse→filter→mount path end to
  end on every run.

## Deferred (needs its own bean + artifacts release)

- Rockchip extlinux.conf emission of `gosd.bootdev`: blocked on a U-Boot
  change to export the booted device under bootstd (and on verifying the
  U-Boot devnum ↔ Linux mmcblk index mapping per board). Until then the
  NanoPi Zero2 stale-eMMC repro is NOT yet fixed on real hardware — this
  PR ships the gosd-init mechanism and its CI coverage only.

## Todos

- [ ] **Hardware verification (JP's bench, NanoPi Zero2)**: flash a stale
      GoSD image to eMMC and a fresh one to SD, confirm today's behavior
      (stale eMMC GOSD-BOOT wins), and check U-Boot's actual env at the
      extlinux prompt (`printenv devtype devnum` after an interrupted
      boot) to confirm the bootstd finding on real hardware before
      scoping the U-Boot-side bean.



---

Closed 2026-08-08 (end-of-session triage): deliverable shipped and on main; status was never flipped from in-progress. Reopen if a hardware sign-off is still outstanding.
