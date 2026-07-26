---
# gosd-1ey5
title: 'pi-zero-w SD card unusable: sdhost DMA address overflow breaks all card I/O'
status: in-progress
type: bug
priority: normal
created_at: 2026-07-26T00:34:16Z
updated_at: 2026-07-26T00:58:45Z
---

Found during Pi Zero W bring-up session 2 (gosd-qltr, 2026-07-26, immediately after the gosd-md4w console fix made dmesg visible). The armv6 kernel detects the SD card (mainline sdhost driver, `sdhost-bcm2835 20202000.mmc: loaded - DMA enabled (>1)`, `mmcblk0: mmc0:aaaa SC16G 14.8 GiB`) but the FIRST read — the partition scan — dies:

    WARNING: CPU: 0 PID: 6 at kernel/dma/direct.h:117 dma_map_phys+0x444/0x46c
    bcm2835-dma 20007000.dma-controller: DMA addr 0xffffffff+4 overflow (mask ffffffff, bus limit 5fffffff).
    ... dma_map_phys ← bcm2835_dma_prep_slave_sg ← bcm2835_request ← mmc_start_request ← ... ← efi_partition ← disk_scan_partitions

Result: `mmcblk0: unable to read partition table` (retries forever, quirks 0x0000c000 fallback included), no /dev/mmcblk0p1/p2, gosd-init fatal-loops on the boot-partition mount. WiFi untestable (gosd-init never gets past the mount). Bonus observation: after the fatal, gosd-init's reboot did NOT take effect — the kernel sat for 10+ minutes accumulating `sdhost wait_transfer_complete - still waiting after 100000 retries` and hung-task warnings, i.e. the wedged controller also blocks/defeats the reboot path (secondary issue, note but don't chase yet).

Bench workarounds attempted and eliminated (2026-07-26, ~01:30):
- `bcm2835_sdhost.force_pio=1`: no-op — the bound driver is MAINLINE drivers/mmc/host/bcm2835.c (platform name "sdhost-bcm2835", the "loaded - DMA enabled (>1)" print at line 1419 of the pinned tree), which has NO module parameters.
- `initcall_blacklist=bcm2835_dma_init`: worse — mainline sdhost -EPROBE_DEFERs forever waiting for its DMA channel; no card detect at all. There is no cmdline-level PIO fallback for the mainline driver.

Fix directions to research (in likelihood order):
1. Switch the pi-zero-w kernel to the DOWNSTREAM sdhost driver (CONFIG_MMC_BCM2835_SDHOST, drivers/mmc/host/bcm2835_sdhost.c) — what RPiOS ships on armv6; has genuine PIO fallback + different (proven) DMA code. Check: DT compatible binding vs the mainline driver when both are =y (link order? blacklist mainline?), and what the bcm2835-rpi-zero-w.dtb sdhost node's compatible string is at the pinned commit. Note our recorded config has BOTH CONFIG_MMC_BCM2835=y (mainline, currently winning) and CONFIG_MMC_BCM2835_MMC=y.
2. Root-cause the mainline path: `dma_map_phys` at kernel/dma/direct.h:117 with addr 0xffffffff smells like a 6.18-era dma_map_phys API conversion bug on 32-bit ARM (highmem/bounce or sg-page-to-phys conversion) — search raspberrypi/linux issues + lore for known regressions; if a small upstream/downstream fix exists, carry it via the kernelspec patch mechanism.
3. Whatever the fix, it lands in build/boards/pi-zero-w (fragment or patch) + local gosd build-kernel validation on the bench, THEN the artifacts-release dance (batch with gosd-36yy window per project convention).

Evidence captures (session scratchpad): qltr-boot-04-fixed.raw (full backtrace, console working), qltr-boot-05-pio.raw (force_pio no-op), qltr-boot-06-nodma.raw (probe deferral). gosd-qltr remains blocked on this bean (console half is FIXED and bench-proven: gosd-md4w's RUNTIME_UARTS=1 kernel registered ttyS0 cleanly this session).

## Research findings (2026-07-26, desk — pinned tree raspberrypi/linux @ 63598c83153e)

**Root cause: our DTB is the odd one out, not the driver.** The rpi downstream
tree has, since 2023, used a slave-DMA convention where MMC/DMA clients hand
the DMA core CPU *physical* addresses and rely on the DT's `dma-ranges` to
translate them to VideoCore bus addresses:

- `drivers/mmc/host/bcm2835.c:1471` — `host->phys_addr = iomem->start`
  (downstream commit c03fe8928bd3 "mmc: bcm2835: Use phys addresses for slave
  DMA config", Phil Elwell; mainline still uses `be32_to_cpup(of_get_address(...))`,
  i.e. the raw 0x7e... bus address, at v6.18's bcm2835.c:1400).
- `drivers/dma/bcm2835-dma.c:1026/:1032` — `bcm2835_dma_prep_slave_sg` maps the
  4-byte FIFO register with `dma_map_resource(..., DMA_SLAVE_BUSWIDTH_4_BYTES, ...)`
  (that's the `+4` in the WARN). The result is NOT checked for
  `DMA_MAPPING_ERROR` — a failed mapping is programmed into the control block
  as-is.
- `kernel/dma/direct.h:96-99` (pin) — the rpi tree carries downstream commit
  372f4e66dad6 "dma-mapping: Use any dma_range_map for phys to DMA" (Phil
  Elwell, 2025-11-17), which makes `dma_direct_map_phys(DMA_ATTR_MMIO)` run
  the address through `phys_to_dma()`/the device's `dma_range_map`. Upstream
  v6.18 identity-maps MMIO (`dma_addr = phys`) instead.
- Our shipped `bcm2835-rpi-zero-w.dtb` is built from the *mainline-style*
  `bcm2835.dtsi`, whose soc node has `dma-ranges = <0x40000000 0x00000000
  0x20000000>` — RAM only (hence "bus limit 5fffffff" = 0x40000000+512MB-1).
  The FIFO's phys addr 0x2020_2040 isn't covered, so `phys_to_dma()` returns
  `DMA_MAPPING_ERROR` = 0xffffffff on 32-bit — that's the literal "DMA addr
  0xffffffff+4 overflow" at `kernel/dma/direct.h:117` (`dma_direct_map_phys`'s
  err_overflow WARN). The unchecked error then lands in the DMA control
  block, the transfer never completes, and sdhost wedges
  (wait_transfer_complete retries forever).
- The downstream DTs RPiOS actually boots carry the missing piece:
  `bcm2708.dtsi:12-15` overrides the soc node with
  `dma-ranges = <0x80000000 0x00000000 0x20000000>, <0x7e000000 0x20000000
  0x02000000>` — the second entry is the VideoCore *peripheral window* that
  translates 0x2020_2040 → 0x7e20_2040. That's why RPiOS (and our own
  pi-zero-2w, which ships the downstream bcm2710 DT with the SAME mainline
  sdhost driver) works and we don't.

**Known-regression trail:** raspberrypi/linux issue #7136 ("6.18: RPi3B+
can't boot from SD card, sdhost-bcm2835 doesn't complete transfer",
https://github.com/raspberrypi/linux/issues/7136) is the same failure class,
diagnosed by Phil Elwell against Leon Romanovsky's 6.18 phys-API series
(e53d29f957b3, f7326196a781: `dma_map_resource` → `dma_map_phys(DMA_ATTR_MMIO)`
with identity mapping). Their fix — 19a2aa9921b3, re-landed as 372f4e66dad6 —
is ALREADY in our pin; it repairs the downstream DTs but leaves the
mainline-style bcm2835 DTBs broken (their dma-ranges never had the peripheral
window; nobody but us boots those DTBs on this tree). No post-pin fix exists
on rpi-6.18.y for bcm2835.dtsi or the dma driver (checked 2026-07-26), and
upstream mainline is unaffected (mainline driver + identity MMIO mapping).

**Fix directions from the original triage, resolved:**
1. Downstream sdhost driver swap: DEAD — `bcm2835-sdhost.c` /
   `CONFIG_MMC_BCM2835_SDHOST` no longer exist at the pin; the tree's only
   sdhost driver is mainline `bcm2835.c` (binds `brcm,bcm2835-sdhost`), which
   is what RPiOS itself now ships. `CONFIG_MMC_BCM2835_MMC` (`bcm2835-mmc.c`)
   binds only the downstream-DT-only `brcm,bcm2835-mmc` compatible — dead code
   with our DTB, no bind ambiguity either way.
2. Root-cause + carried patch: CHOSEN — see Fix below.
3. Config-level DMA avoidance: DEAD — bench-proven (no mainline force_pio
   param; blacklisting the DMA controller probe-defers sdhost forever).

**Secondary (reboot blocked by wedged sdhost, noted not chased):** the
partition-scan read is programmed with the bogus 0xffffffff source address, so
the DMA never signals completion and the request never leaves the block
layer — sdhost spins `wait_transfer_complete` (100k retries per attempt) while
the mmc core retries the read indefinitely. gosd-init's reboot(2) then drives
kernel_restart → device/disk shutdown, which waits on that forever-in-flight
IO — hence the 10+ minutes of hung-task warnings instead of a reset. Any fix
that makes the mapping succeed (or fail the request cleanly) dissolves this;
a defensive `dma_mapping_error()` check in `bcm2835_dma_prep_slave_sg` would
be an upstream-able hardening for the rpi tree, but is not carried here.

**Related discovery for the WiFi milestone (out of scope here):** our
mainline-style DT puts the 43430's SDIO on the `&sdhci` node
(`brcm,bcm2835-sdhci`), whose only driver in this tree is mainline
`sdhci-iproc` — and our kernel.config has `CONFIG_MMC_SDHCI_IPROC is not set`
(bcmrpi_defconfig relies on the downstream `bcm2835-mmc` driver, which can't
bind our DT). WiFi will need `CONFIG_MMC_SDHCI_IPROC=y` in the fragment once
SD I/O is proven; flagging now so the next bench session isn't surprised.

## Fix

Carry a one-hunk DTS patch via the kernelspec Patch mechanism:
`build/boards/pi-zero-w/kernel/patches/0001-soc-peripheral-dma-ranges.patch`
adds the VideoCore peripheral window `<0x7e000000 0x20000000 0x02000000>` as a
second `dma-ranges` entry on `bcm2835.dtsi`'s soc node — byte-for-byte the
window `bcm2708.dtsi` already carries (the RAM alias entry is left untouched).
With it, `phys_to_dma(0x20202040)` → 0x7e202040, exactly the CB address
RPiOS's own DT produces; RAM translations, allocation limits and every other
board are unchanged (each board's DTSPatches apply only to its own build).
Verified `patch -p1 --forward` applies cleanly against the pinned
`bcm2835.dtsi`.

Why the alternatives lost: the driver swap is impossible (driver removed
upstream-of-us; see findings) and shipping the downstream
`bcm2708-rpi-zero-w.dtb` instead would fix this too but swaps the whole DT
mid-bring-up (renames the artifact, invalidates session evidence, drags in
downstream-only nodes/overrides we haven't audited) — a much larger blast
radius for the same mechanism. The patch fails loudly (build abort) if a
future pin changes that dtsi line, forcing a review exactly when one is
needed; if rpi ever adds the window themselves, the patch conflicts and gets
dropped.

No config/fragment change: the committed kernel.config is untouched by a DTS
patch. internal/artifacts.Version untouched per the release convention — this
reaches real builds only after the next artifacts/vX.Y.Z release (batch with
the gosd-36yy window).

**Bench validation (blocked on hardware session):**
- [ ] Local `gosd build-kernel --board pi-zero-w` rebuild (DTS patch is a
      cache-key change → real rebuild) + `dtc -I dtb -O dts` spot-check that
      the built DTB's soc node carries both dma-ranges entries
- [ ] Flash + boot: no `DMA addr ... overflow` WARN; `mmcblk0p1/p2` appear;
      gosd-init mounts GOSD-BOOT
- [ ] WiFi test reachable (expected next wall: CONFIG_MMC_SDHCI_IPROC, see
      findings) and hello.local responds
- [ ] 5x power-cycle stability per gosd-qltr checklist
