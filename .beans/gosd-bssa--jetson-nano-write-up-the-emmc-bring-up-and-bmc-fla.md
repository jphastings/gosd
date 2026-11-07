---
# gosd-bssa
title: 'Jetson Nano: write up the eMMC bring-up and BMC flashing procedure'
status: completed
type: task
priority: normal
created_at: 2026-11-07T20:59:57Z
updated_at: 2026-11-07T21:02:31Z
parent: gosd-72je
---

Record the 2026-11-07 bench session on the Jetson Nano eMMC module (in a Turing Pi 2) and JP's decisions from it:

- the eMMC module only;
- `/data` on the NVMe;
- the BMC flashing procedure documented now, with a Go replacement queued.

**Summary of Changes**

- `docs/jetson-nano-flashing.md` (new): flashing a Nano eMMC module from a Turing Pi 2's BMC with NVIDIA's offline massflash blob. Covers the space it needs on the BMC's microSD card, preparing the blob on x86_64 or arm64 Linux, and why a Mac VM won't do.
- `scripts/jetson-nano-flash-from-bmc.sh` (new): the BMC-side script the guide uses. It wraps NVIDIA's three x86 tools to run under `qemu-i386`, puts the node in recovery mode on the BMC's USB, and runs NVIDIA's own `nvmflash.sh`.
- `docs/development/jetson-nano.md`: retargeted to the eMMC module, with the bench findings. Dev-kit-only material (QSPI, J50/J44 pins, the 2GB kit) is dropped.
- `docs/development/turing-pi-2-bmc.md`:
  - `tpi` on the BMC needs no login;
  - `ttyS<N>` is node N on BMC firmware 2.1.0 and later;
  - `tpi uart get` returns the whole buffer;
  - a Jetson's console is only on the UART2 DEBUG headers;
  - the USB routing modes for a Jetson;
  - the BMC's storage.
- CLAUDE.md: the `jetson-nano` board-ID entry now names the eMMC module and the flashing guide.
- Beans:
  - epic gosd-72je rewritten for the eMMC module;
  - gosd-9o0m retargeted, with the proven steps checked off;
  - gosd-uw32's docs requirement updated;
  - new: gosd-wxy7 (eMMC controller in the DTB), gosd-j18s (`/data` on NVMe), gosd-32dw (flashing in Go).
