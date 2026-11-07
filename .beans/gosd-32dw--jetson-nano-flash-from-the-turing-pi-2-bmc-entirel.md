---
# gosd-32dw
title: 'Jetson Nano: flash from the Turing Pi 2 BMC entirely in Go'
status: todo
type: feature
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-07T20:59:10Z
parent: gosd-72je
---

Replace the procedure in `docs/jetson-nano-flashing.md` with one static Go binary that takes a gosd-built image and a node number and flashes it. That procedure is:

1. On a Linux host, NVIDIA's Python `nvmassflashgen.sh` signs everything offline.
2. On the Turing Pi 2's BMC, NVIDIA's 32-bit x86 tools run under `qemu-i386`.

The binary should be `CGO_ENABLED=0`: `GOARCH=arm GOARM=7` for the BMC, and any Linux host as well. Every piece below was observed working on the bench on 2026-11-07.

- **Inputs.** Download NVIDIA's BSP from NVIDIA's own URL at a pinned sha256. Never re-host it (the third-party-blob decision). Today that's `Jetson-210_Linux_R32.7.6_aarch64.tbz2`, the final T210 release.
- **Offline signing, as `nvmassflashgen.sh` does it.** The module has production fuses with no PKC: the tools say "Assuming zero filled SBK key". That covers:
  - the zero-key signing done by `tegrasign`;
  - `tegrahost`'s signature headers;
  - `tegrabct`'s BCT and BFS updates;
  - the boot ROM messages (`tegrarcm --listrcm` and `--updatesig`);
  - the partition images (`flash.xml.bin`, `crc-flash.xml.bin`, `gpt.bin`).
- **The USB sequence, from NVIDIA's `nvaflash.sh`:**
  1. Read the boot ROM's 16-byte UID.
  2. Send the RCM messages.
  3. Download the BCT.
  4. Download `ebt` (cboot) and `rp1` (DTB).
  5. Boot recovery.
  6. Query cboot's "3p server" for platform details.
  7. Create the partitions.
  8. Write the BCT last.
  9. Cold-boot.

  `QEMU_STRACE` on NVIDIA's tools shows the framing: bulk endpoints 0x01 and 0x81, 12-byte nv3p headers, 4-byte checksums and 16 KiB data chunks. NVIDIA's open-source tegrarcm (github.com/NVIDIA/tegrarcm: `rcm.c`, `nv3p.c`) documents the earlier generation of both protocols.
- **USB access.** Use usbfs ioctls directly (`USBDEVFS_CLAIMINTERFACE`, `USBDEVFS_BULK`). BMC firmware 2.3.4 exposes `/dev/bus/usb`, and the Nano's recovery device enumerates on the BMC under `tpi usb flash -n N --bmc`.
- **Timing.** The boot ROM applet and cboot only wait a short while for the host. A host about 4 s slow between the BCT and `ebt` failed every time, and one about 0.6 s slow failed intermittently. So keep the per-step work out of the gaps between steps.

**Open questions:**
- Whether NVIDIA's BSP licence permits automated download and use this way.
- Where the binary lives: a `gosd` subcommand, or a separate binary copied to the BMC (a `tpi flash`-like experience).
- Whether one blob per app is worth caching. NVIDIA's chain is identical for every app; only `APP` differs.
