---
# gosd-84b8
title: 'cubie-a5e: image does not boot on the 1GB RAM variant (SPL DRAM init fails)'
status: in-progress
type: bug
priority: normal
created_at: 2026-08-16T18:44:19Z
updated_at: 2026-08-16T18:45:39Z
parent: gosd-h1wv
blocking:
    - gosd-6pfn
---

The Cubie A5E's first bench boot (bean gosd-6pfn) fails in U-Boot SPL, before
the kernel, on JP's **1GB LPDDR4x** board. Deterministic across cold boots,
identical every time:

```
U-Boot SPL 2026.04 (Aug 07 2026 - 23:50:00 +0000)
DRAM:DRAM test failure at address 0x6fffffc0
 0 MiB
### ERROR ### Please RESET the board ###
```

## What is NOT wrong

Everything gosd itself contributes is proven correct by this very log:

- The BootROM found our SPL, so the **single raw write at byte 8192 is right**
  (the epic's central new-to-gosd boot-chain assumption, verified on hardware).
- The SPL that runs is **ours** (banner build date matches the v0.10.0
  artifact), so the build pipeline, FIT assembly and flashing path all work.
- Console **ttyS0 @ 115200** is correct and reads cleanly on the CP2102N — no
  Rockchip-style garble.
- TF-A is not implicated: this is SPL, before BL31 is loaded from the FIT.
- Our config fragment sets only `CONFIG_BOOTDELAY=0`; we override nothing
  DRAM-related. The failure is stock mainline behaviour.

## Root cause

`mctl_calc_size()` (arch/arm/mach-sunxi/dram_dw_helpers.c) decodes the address
exactly: `0x6fffffc0 = 0x40000000 + (1024MiB x 3/4) - 64`. So auto-detection
sized the array at 1GiB — correct for this board — and then both the
768MiB check and its -64B fallback failed to read back a written pattern,
so it returns 0 and SPL halts.

Sequence in `sunxi_dram_init()`: geometry is detected at a conservative
`clk=360` with the driver's own safe LPDDR4 timings, then the controller is
re-initialised at `CONFIG_DRAM_CLK` (**1200** by Kconfig default for
MACH_SUN55I_A523) using the defconfig's `TPR11`/`TPR12` before the size check
runs. No `panic("This DRAM setup is currently not supported")` appears, so
training succeeded — this is a data-integrity failure at the top of the range
under the tuned parameters, not a dead controller.

`radxa-cubie-a5e_defconfig` carries ONE fixed set of values that U-Boot's own
Kconfig calls "value from vendor DRAM settings" — per-chip calibration data.
**The 1/2/4GB Cubie A5E variants ship different DRAM chips**, and the upstream
values came from whichever reference unit upstreamed the board.

## Corroboration

- Armbian forum, "RADXA Cubie A5E 1GB RAM Armbian CLI stucks while uboot via
  sdcard": a 1GB unit fails on mainline-derived U-Boot while Radxa's own image
  boots on the same board; thread attributes it to the 1GB version using a
  different DRAM chip. A community fix reads `tpr6/tpr10/tpr11/tpr12` back off
  the working vendor bootloader
  (`Guation/radxa-cubie-a5e-armbian-build@202f1bf`) — exactly the fields our
  defconfig hardcodes.
- armbian/build#9764 — another Cubie A5E unit, same family of failure.

## No newer pin fixes this

`dram_sun55i_a523.c` and `dram_dw_helpers.c` have no commits after Oct 2025
(both fixes already ancestors of our v2026.04 pin). The three post-v2026.04
defconfig commits (SPI, power LEDs, gmac1) touch no `CONFIG_DRAM_SUNXI_*`
value. **Bumping the U-Boot tag alone cannot fix this.**

## The decision this forces (for JP)

One gosd image per board must boot every RAM variant a buyer might own. If the
vendor DRAM parameters really are per-variant, the options are:

1. Ship the 1GB parameters and re-verify on a 2GB/4GB board (needs hardware we
   don't have).
2. Find parameters that work across variants (e.g. backing `CONFIG_DRAM_CLK`
   off 1200), trading bandwidth for compatibility.
3. Document cubie-a5e as supporting only the variants we can prove, and fail
   loudly rather than looking like a brick.
4. Upstream a runtime variant probe — the real fix, but upstream work.

This is a locked-decision-level call (the epic's "mainline-only" rule bears on
options 1 and 2), so it is written up here rather than decided.

## Todos

- [ ] Bench-test the community 1GB parameters as a config fragment (in
      progress at time of writing) to confirm the diagnosis
- [ ] JP decides which of the four options above cubie-a5e takes
- [ ] COMPATIBILITY.md: record hardware-verified status honestly for this board
