---
# gosd-32dw
title: 'jetson-flash: flash a Jetson Nano eMMC module without NVIDIA''s tools'
status: in-progress
type: feature
priority: normal
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-08T21:18:41Z
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
- **End to end with the protocol fixed (2026-11-08).** With the probing flags gone, `jetson-flash spike` went from boot ROM to CBoot's flashing server answering the version query in 3.7 s, first time. Two things to know before parsing CBoot's replies:
  - CBoot's STATUS 0 after the version query carries an empty message followed by about 300 bytes of what looks like uninitialised memory, so read the message only up to its first NUL.
  - nvtboot logs `Error in NvTbootGetTOSBinaryLength: 0x11 !` because no TOS image is sent, and carries on into the flashing server regardless.
  - The `0x05`/`0x06` probes left the eMMC untouched: the module booted GoSD from it normally afterwards.

## CBoot's flashing server (second PR)

- [x] Map the server's commands by experiment (`jetson-flash probe`, hidden).
- [x] Write `APP` and cold-boot into it.
- [x] A `jetson-flash` command that reflashes `APP` from a gosd `.img`: `jetson-flash flash`. On the bench (2026-11-08, node 3) it reflashed hello A to hello B in 36 s from power-on, reading `APP` straight out of the `.img` with the first flash's blob for the boot chain, and the module came up as hello B (`loglevel=8`).
- [x] Offline signing in Go, so no NVIDIA tool runs at all.
  - `flash` builds the boot chain and every partition from NVIDIA's L4T kit with `internal/l4t` (beans gosd-9ned and gosd-le3i); the blob-dir argument, the blob loader and `spike` are gone.
- [x] A first flash of a module NVIDIA's layout isn't on yet: the partition table, every partition, the BCT last.
  - Every `flash` is now a first flash, in the order the bench proved step by step; its first run as one command is still to come.
- [ ] Licence acceptance, and releases for `linux/arm`, `linux/arm64` and `linux/amd64`.
  - Acceptance is done (third PR, below). Releases are a later PR.

### Bench findings (2026-11-08, node 3)

Found by device experiments alone, reading CBoot's console as they ran. No NVIDIA tool was traced.

- **The framing is the applet's.** Each session opens with the version query, and STATUS takes two ACKs. A successful STATUS's message is uninitialised memory too, so only its code means anything.
- **Commands:**
  - `0x02` read: args are the partition name, NUL-terminated. The device answers `CMD 0x08 {8}`, a DATA holding the partition's u64 size, then the contents as DATA (EKS's 80 KiB came as one packet), then STATUS 0.
  - `0x03` write: args are exactly `{char name[40]; u64 size}`. The device answers `CMD 0x08 {0}` and takes DATA packets of up to 1 MiB, each ACKed, then sends STATUS 0 (`partition <name> write successful`). A size larger than the partition is refused; a smaller one is fine.
  - `0x04` erase a partition by name. `0x05` runs OEM commands (args not yet known). `0x06` with u32 `0` cold-boots: the device drops off USB, and boots from the eMMC even while the BMC still holds the recovery strap.
  - `0x0a` flushes to storage. `0x0c` skips the eMMC sanitize. `0x0b` looks like an eMMC firmware update (`firmware size not 512 aligned`) and was left alone. `0x07`–`0x09` and `0x0d`–`0x20` are "unknown command".
- **Args longer than a command's struct hang the server.** It ACKs them, then logs `Command receive is failed with error 0x120000` and `hanging in flashing server`, and needs a power cycle. Shorter args are padded with whatever was in its buffer, which is how an empty write revealed its size field.
- **Reflashing is one write.** Writing hello A's 256 MiB boot partition to `APP` over hello B took 33 s (about 8 MB/s). A cold boot then came up as hello A (identity `43f466623cf1`, `loglevel=7`). So a module NVIDIA's layout is already on needs only the boot chain, whose signed files are the same for every app, one `APP` write and a cold boot.
- Reading `EKS`, writing the same bytes back and reading it again round-tripped byte for byte. CBoot's `EKS magic header mismatch` warning predates the write.

## A whole first flash, from NVIDIA's kit (third PR)

**Never send CBoot an OEM command (`0x05`) of any type but 1.** Type 9 provisions the eMMC's RPMB key, which can never be changed afterwards; a zero-argument probe did exactly that to node 3's module (2026-11-08). `flash` sends only the storage query, as fixed bytes nothing can vary: type 1 at offset 0, sub-command 1 at offset 4, zeroes to 48 bytes. A test pins them.

### Decisions (JP, 2026-11-08)

- **Every `flash` is a full first flash**, one code path, with no detection of an existing layout. In order, each write in a session of its own:
  1. Boot CBoot's flashing server with the Go-built boot chain.
  2. Query the eMMC's size.
  3. Write `GPT`, which erases the eMMC.
  4. Write every partition in NVIDIA's order, with `APP` taken from the image's MBR partition 1.
  5. Flush. This is **pending bench confirmation**: CBoot hasn't yet been seen to take a write after a flush.
  6. Write the BCT.
  7. Flush, then cold-boot.
- **Kit source.** By default `flash` downloads `l4t.R32_7_6` and streams it, never writing the 279 MB kit to disk. `--kit <file.tbz2>` reads a local copy instead. `Kit.Download` became `Kit.Open`, so the caller can watch the stream's progress.
- **Licence.** On a cache miss that needs a download, `flash` shows `Kit.LicenseURL` and asks `[y/N]` on a terminal. `--accept-nvidia-license` skips the prompt. With no terminal and no flag it stops, naming the flag and the URL. `--kit` and a cache hit never ask: a `--kit` copy came from NVIDIA's own page.
- **Cache.** The kit's extracted members (about 2.4 MB) live under `os.UserCacheDir()/gosd/l4t/<kit sha256>/`. They are the user's own download, never redistributed. A copy is written and synced in a temporary directory, then renamed into place, so a write cut short is never read back as complete. Each flash prunes everything else there, best-effort, and a failed cache write only prints a note.
- **Every member has a pinned sha256** in `l4t.R32_7_6.Members`, keyed by its path in the kit, after gosd-1jjh's precedent. `Kit.Read` checks the pins behind the kit's own sha256. Every cache read checks them too. A cached copy with a member missing or mismatched is deleted, never used: `flash` prints one line saying why, and falls back to `--kit` or a download, with the licence prompt.
- **`probe` refuses OEM command `0x05`** unless its args start with type 1, at parse time, before anything is sent. A range covering `0x05` is refused too. CBoot pads short args with whatever its buffer held, so the type can't be left out.
- **Fail fast before USB.** Before it waits for the module, `flash` checks that the image has an MBR, isn't truncated and that its boot partition fits `APP` (14 GiB). It also gets the kit, from the cache, the licence prompt and a download, or `--kit`, and builds the boot chain.
- **Each partition write and each flush must end in CBoot's STATUS 0** within 30 s. `drain` used to treat silence as success. On the bench every write's and the flush's reply came well within that, as the UART's half-second gaps between steps show.

### Crash ordering

Every flash is a whole first flash, so an interrupted one is recovered by flashing again from the start. The order only has to keep a half-written eMMC from booting:
- The `GPT` write erases the boot device, where the BCT lives, before it sanitizes the user area and writes the GPTs. From then on the eMMC has no BCT.
- Every other write is confirmed by CBoot and then flushed before the BCT is written. So the BCT can't reach the eMMC ahead of anything it describes, however CBoot buffers writes.
- The BCT is flushed in turn. Until it lands, the eMMC has no BCT, and the boot ROM boots nothing from it. A BCT cut short fails its own CMAC, which the boot ROM presumably treats the same way.
- Recovery mode belongs to the boot ROM, which enters it on the recovery strap, so `jetson-flash flash` can always start over.

**What this rests on is observed, not documented by NVIDIA:**
- CBoot's console showed the erase order (2026-11-08).
- A blank module showed nothing on power-on, and entered recovery mode on the strap (bean gosd-9o0m).
- We haven't seen the boot ROM fall back to recovery mode by itself, and nothing relies on it.
- **The flush before the BCT is pending bench confirmation.** CBoot hasn't yet been seen to take a write after a flush. Without that flush, a power cut between the BCT's STATUS and the final flush could leave the BCT on the eMMC without everything before it.

The same argument is in `internal/jetsonflash`'s `flash` doc.
