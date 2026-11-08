---
# gosd-le3i
title: 'jetson-flash: build every first-flash partition from NVIDIA''s kit in Go'
status: completed
type: task
priority: normal
created_at: 2026-11-08T12:24:40Z
updated_at: 2026-11-08T12:47:32Z
parent: gosd-72je
---

A module that has never had NVIDIA's layout on it needs a first flash: every partition NVIDIA's own flash writes, not just `APP`. Bean gosd-9ned built the five boot-chain files from NVIDIA's L4T kit. This bean builds everything else a first flash writes, from the same kit, so `jetson-flash` (bean gosd-32dw) never needs NVIDIA's tools, not even once. Wiring it into `jetson-flash flash` comes after gosd-32dw's PRs and gosd-9ned's PR merge.

## Decisions

- **One fixed layout for every app.** NVIDIA's `flash_l4t_t210_emmc_p3448.xml` from the kit, with `APPSIZE` at NVIDIA's default `ROOTFSSIZE` of 14 GiB. UDA takes the rest of the eMMC. GoSD writes its boot volume into `APP`, so any `--boot-size` fits, and nothing about the layout varies per app.
- **Fresh GUIDs every flash.** Random v4 GUIDs, as NVIDIA's tools produce, with each GPT entry reusing its partition-table GUID.
- **The partition table's 81 host-pointer words carry the offsets of their strings.** NVIDIA's tools dump raw 32-bit x86 heap pointers there. The device ignores those words: it accepted our table and booted (bench, below).
- **The version file's board line is fixed at `BOARDID=3448 BOARDSKU=0002 FAB=401`.** NVIDIA reads FAB from each module's EEPROM, but nothing on the boot path is known to read the version file (**L**), so the value is recorded rather than read.
- **Generated on the user's machine, never redistributed**, as for gosd-9ned.

## What the research established (2026-11-08)

Found by comparing NVIDIA's kit files with what NVIDIA's tools wrote for this module (the massflash blob, and the log of the flash that wrote it). NVIDIA's shipped Python and shell were read for behaviour only. No NVIDIA binary was run, disassembled or traced. Python prototypes reproduced NVIDIA's bytes:
- **Unchanged from the kit:** NVC (`t210ref/nvtboot.bin`), EKS, BMP, RP4.
- **Same signing as `cboot.bin`:** TBC (`nvtboot_cpu.bin`).
- **Their own headers, reproduced byte for byte:** WB0 (`warmboot.bin`), BPF (`sc7entry-firmware.bin`, wrapped like the DTB), TOS (the image plus a CMAC appended) and LNX (an Android boot image of `p3450-0000/u-boot.bin`, with a signature header embedded at 0x5A0).
- **Reproduced exactly:** the layout, `gpt.bin` (the primary GPT from LBA 1, user area only, sized from the eMMC's sector count) and the version file (CRC is POSIX `cksum`).
- **The partition table:** exact apart from the 81 pointer words and the CRC over them.
- **The final BCT, exact:** two bootloader entries for NVC, and the boot-file (BFS) and kernel-file (KFS) info. Each SUM is the XOR of the little-endian u32 words of the group's partition files, each zero-padded to its partition size.

## What the bench established (2026-11-08, node 3)

- **There's no separate partition-table upload.** CBoot's flashing server treats a write to the partition named `GPT` as the setup step. It erases the boot device, sanitizes the user area, writes the protective MBR and both GPTs from `gpt.bin`, then re-reads the GPT. Every other partition, `PT` included, is an ordinary named write, and the BCT is written last.
- **The eMMC's size comes from OEM command `0x05`.** Arguments are the shared 48-byte buffer, type u32 1 (platform details), then sub-command u32 1 (storage). The reply is 32 bytes: `{u64 0; u32 sectors; u32 sector size}` for the boot device (16384 × 512) and then for the user area (30777344 × 512 on this module).
- **OEM type 9 provisions the eMMC's RPMB rollback key. Never send it.** An RPMB key can be set only once. A zero-argument probe of type 9 did exactly that on this module (2026-11-08). Type 15 updates the rollback counter (skipped on this unfused module). Send no OEM type other than 1.
- **End to end.**
  - The prototype-generated set was written in NVIDIA's order: `GPT`, `PT`, `PT-1`, NVC through BPF, their `-1` copies, VER_b, VER, `APP`, DTB, TOS, EKS, LNX, their `-1` copies, BMP, RP4, then the BCT, flush, cold boot. It used fresh GUIDs, offset pointers, the 14 GiB APP and hello A's boot volume.
  - The module booted BFS0 and logged `PT crc32 and magic check passed`, then ran hello A up to `listening on [::]:80`.
  - Its CBoot warnings (`PT: Partition LNX NOT found`, EEPROM read failures, `trusty_fast`) appear in every one of the 15 earlier boots that NVIDIA's own flashes produced.

## Todo

- [x] `internal/l4t`: read the further kit members, and build every first-flash partition, the partition table, `gpt.bin`, the version file and the final BCT, in write order.
- [x] Tests: the rules on synthetic inputs, plus local-only gated tests proving byte identity with NVIDIA's output.
- [x] Record the bench findings, and the RPMB warning, in the Jetson Nano development notes.

## Summary of Changes

- `internal/l4t` reads ten more kit members (`Inputs` gains `NVTBoot`, `NVTBootCPU`, `WarmBoot`, `SC7Entry`, `TOS`, `EKS`, `BMP`, `RP4`, `UBoot` and the `Layout` template), still in one streamed pass.
- `BuildFirstFlash(in, Storage{BootSectors, UserSectors}, now, random)` returns a `FirstFlash`: `GPT` (written first), `Partitions` as `{Name, Data}` in NVIDIA's write order with `APP`'s data left nil for the caller, and the final `BCT` (written last). EBT and RP1 reuse `Build`'s signed CBoot and DTB.
- The layout is the kit's template with flash.sh's substitutions for this module and APP at 14 GiB. The KFS sector offset is the reported boot-device size, not a constant. GUIDs come from `random`: one per partition in layout order, then the disk's, each stamped v4.
- Fails fast, naming the partition, when the reported eMMC is too small for the layout, a kit file has an unexpected shape or outgrows its partition, or the layout names a file gosd does not build.
- Tests: synthetic-input tests for the write order, GPT placement and CRCs (LastUsable N−33, UDA to N−34), PT pointer offsets and GUID reuse, BCT entries and BFS/KFS sums over zero-padded files, the signed files, POSIX `cksum` vectors and the refusals. With `GOSD_L4T_KIT` and `GOSD_L4T_BLOB` set, a gated test feeds NVIDIA's own GUIDs and timestamp back in. It proves `gpt.bin` and every partition file identical to NVIDIA's, the partition table identical once its 81 pointer words are swapped for NVIDIA's, and the BCT identical when built over NVIDIA's partition table.
- Fed the GUIDs and timestamp of the set that booted on the bench, the output was identical to that set, partition table and BCT included (checked locally, not committed).
- The Jetson Nano development notes gain a section on a first flash without NVIDIA's tools: the `GPT` write as the setup step, the storage query, the warning never to send OEM type 9 (it provisioned this module's RPMB key), and which CBoot warnings are normal.
