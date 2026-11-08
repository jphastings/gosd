---
# gosd-32dw
title: 'jetson-flash: flash a Jetson Nano eMMC module without NVIDIA''s tools'
status: in-progress
type: feature
priority: normal
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-07T23:37:18Z
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


## Decisions (JP, 2026-11-07)

- **A separate binary, `jetson-flash`, not a `gosd` subcommand.** It's released next to `gosd`, for people more advanced than the usual "download an `.img`, flash an SD card" audience, who may still have no Go toolchain. So it ships prebuilt and static. It runs where the Nano's recovery USB is: on a Turing Pi 2 that's the BMC (`linux/arm`, ARMv7), otherwise any Linux PC cabled to the recovery port (`linux/arm64`, `linux/amd64`). Its input is the `.img` that `gosd build --board jetson-nano` produces; the image's boot partition becomes `APP`. Per JP's Go-CLI conventions: `cmd/jetson-flash/` holds only `main` and the Cobra commands, and the logic lives in `internal/`.
- **Clean-room protocol work, in order:**
  1. NVIDIA's open-source `tegrarcm` (`nv3p.c`), plus experiments against the device.
  2. Only if that stalls: watching NVIDIA's tools' USB traffic while they run.

  Never disassembling NVIDIA's binaries, which clause 2.2 of their licence forbids.
- **Licence handling:** signed images are generated on the user's machine and never redistributed (clauses 1.1(d) and 2.3). `jetson-flash` asks the user to accept NVIDIA's licence before its first download of NVIDIA's kit.
- **Research spec** (boot ROM handshake, `nv3p` framing, zero-key AES-CMAC signing, verified against NVIDIA's own signed files): see the 2026-11-07 research notes summarised in the first spike PR.

## Spike (first PR)

- [x] A static Go helper on the BMC. It reads the boot ROM's UID, sends the signed `rcm_0`/`rcm_1` messages (expecting `0x00210001` and `0` back), and the recovery applet runs.
- [x] It drives the applet over `nv3p` to download the BCT, then `ebt` (cboot) and `rp1` (DTB), then boots into recovery, using files NVIDIA's tools already signed. CBoot answers.

### Bench findings (2026-11-07, node 4)

- **Boot ROM: proven.** `jetson-flash spike` on the BMC read the 16-byte UID (BR_CID `0x32101001643c270718000000150100c0`, identical to NVIDIA's tool's), sent `rcm_0_encrypt.rcm` (one 1032-byte write) and got `01 00 21 00`, then sent `rcm_1_encrypt.rcm` (4 KiB writes) and got `00 00 00 00`. The applet logged `Processing in recovery mode` and `Established communication link with host`. The device kept its enumeration (same sysfs node, `2-1`).
- **T210's nv3p is version 2, not tegrarcm's 1.** Sent a version-1 CMD header, the applet stopped reading after the 12-byte header and answered `02 00 00 00 05 00 00 00 00 00 00 00`: a NACK in protocol version 2.
- **What T210's nv3p turned out to be (2026-11-08, found by device experiments alone; no NVIDIA tool was traced):**
  - Packets: header `{u32 version = 2, u32 type, u32 seq}`. CMD and DATA then carry a u32 length (for a CMD it counts the command word too), the body, and a u32 checksum. The checksum is the negated byte sum of the header and body **but not the length**. Each field goes as its own bulk transfer.
  - After every STATUS the applet takes **two** ACKs. With one, it ACKs the next command and silently drops it.
  - `0x01`: version query. The device answers `CMD 0x08 {16}`, `DATA` (`01 00 00 00 00 00 00 00 01 00 00 00 00 00 00 00` from the applet, `02 …` from CBoot), then `STATUS 0`.
  - `0x08` is how the device announces its own return data; the host never sends it.
  - `0x07` is STATUS: args `{u32 code; char msg[]}`, for example `0x120000 "Unwanted error in 3p server"` or `4 "Unknown command"`.
  - `0x03`: download, with args `{char type[4]; u32 load; u32 entry; u32 length}`. The type is NVIDIA's CLI name: `"bct"`, `"ebt"`, `"rp1"`. Load and entry come from the image's signature header (`0x92c00000`/`0x92c00258` for cboot, zero for the BCT and the DTB). The device answers `CMD 0x08 {0}`; the host ACKs it, sends one DATA packet with the whole file, and gets an ACK, then STATUS 0 (about 220 ms later for the BCT, while it trains SDRAM). UART: `Downloaded BCT successfully`, `Downloaded Bootloader successfully`, `Downloaded rp1 successfully`.
  - `0x09`: boot, with a u32 type. `0` brings up CBoot's flashing server (`Proceeding to flashing Server`, `nv3p_priv_usbf_open USB configuration success`), which answers the same version query. `1` tries to boot a kernel image from RAM. Any other value gets "Boot type is not supported".
  - Others probed in the applet: `0x02` gets "image read is not supported"; `0x05` returns an empty result; `0x06` with a u32 argument made the device drop off USB (`ESHUTDOWN`), presumably a reset; `0x04`, `0x08`, `0x0a`–`0x12` and `0x7f` are "Unknown command".
