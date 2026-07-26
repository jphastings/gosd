---
# gosd-1ey5
title: 'pi-zero-w SD card unusable: sdhost DMA address overflow breaks all card I/O'
status: todo
type: bug
created_at: 2026-07-26T00:34:16Z
updated_at: 2026-07-26T00:34:16Z
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
