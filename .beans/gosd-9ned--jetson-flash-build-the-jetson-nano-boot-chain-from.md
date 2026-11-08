---
# gosd-9ned
title: 'jetson-flash: build the Jetson Nano boot chain from NVIDIA''s kit in Go'
status: in-progress
type: feature
created_at: 2026-11-08T09:08:55Z
updated_at: 2026-11-08T09:08:55Z
parent: gosd-72je
---

`jetson-flash` (bean gosd-32dw) boots a Jetson Nano into CBoot's flashing server with five signed files. Until now they came from the massflash blob that NVIDIA's own tools produce on a Linux host. This bean builds them in Go from NVIDIA's L4T kit, so the user never has to run an NVIDIA tool. Wiring it into `jetson-flash flash` comes after gosd-32dw's PRs (#410, #411) merge, so this PR stays off that stack.

## Decisions

- **Inputs:** NVIDIA's `Jetson-210_Linux_R32.7.6_aarch64.tbz2`, the final T210 release, from NVIDIA's own URL at a pinned sha256 (`865717ee…0212`, 279,353,831 bytes). It is never re-hosted, per the third-party-blob decision, and a user's own copy can be used instead. Four members are read: `bootloader/nvtboot_recovery.bin`, `bootloader/t210ref/cboot.bin`, `bootloader/t210ref/BCT/P3448_A00_lpddr4_204Mhz_P987.cfg` and `kernel/dtb/tegra210-p3448-0002-p3449-0000-b00.dtb`.
- **Streamed, never stored.** The kit is hashed, decompressed and untarred in one pass. Once the four members are in hand, the rest of the stream is only hashed. Nothing is kept unless the whole download matches the pin. That matters on a Turing Pi 2 BMC, whose root filesystem is 24 MB.
- **Signing is zero-key AES-CMAC (RFC 4493), with no encryption.** The module has production fuses and no SBK.
- **The licence prompt belongs to the CLI** and comes with the wiring. Signed output is generated on the user's machine and never redistributed (clauses 1.1(d) and 2.3 of NVIDIA's licence).

## What the bench established (2026-11-08)

Found by comparing NVIDIA's raw kit files with the signed files its tools wrote into the massflash blob. No NVIDIA binary was disassembled or traced. A Python prototype built from these rules reproduced NVIDIA's `rcm_0_encrypt.rcm`, `rcm_1_encrypt.rcm`, `cboot.bin.encrypt` and DTB `.encrypt` byte for byte, and the BCT's whole signed region and CMAC.

- **RCM messages:** the 0x2A8-byte header, payload and padding rules in the research notes (`rcm_0`: opcode 6, no payload; `rcm_1`: opcode 4, payload `nvtboot_recovery.bin`, arg 0x40010000), with `CMAC(0, msg[0x238:])` at 0x128.
- **`cboot.bin`:** the kit's file already reserves its 0x400-byte signature header. Signing writes its length as a u64 at 0x8 and 0x240, then `CMAC(0, file[0x230:])` at 0x118.
- **DTB:** a 0x400-byte header (`DTB\0` at 4; u64 total length at 0x8 and 0x240; the "unspecified" sentinel `0xFEEDDEADBEEFBAB1` for the load address at 0x10 and 0x248 and for the entry at 0x250; 0xAA over 0x18–0x118, 0xEE over 0x118–0x228, 0xCC over 0x230–0x240), then the kit's DTB padded to 16 bytes with 0x80 and zeros, then `CMAC(0, file[0x230:])` at 0x118.
- **BCT (10240 bytes):**
  - The config's four SDRAM sets, 474 fields each, lie at 0x58C in file order with no gaps (stride 0x768); `NvBootMemoryType_LpDdr4` is 3.
  - The rest is constants NVIDIA's tools write for this module:
    - unsigned: 0x200 and 0xE0F at 0x0, 3 at 0x47C, ODM data 0xA4000 at 0x508 (`ODMDATA` in `p3448-0000.conf.common`);
    - signed, from 0x530: 0x00210001, 14, 9, 0x1000000, 1, 4, 9, 2;
    - 0x80 at 0x27EC.
  - `CMAC(0, bct[0x510:0x2800])` goes at 0x310.
  - NVIDIA's tools also write two bootloader entries at 0x232C, describing `nvtboot` in boot0, and boot-file-set data into the unsigned customer area. Booting into recovery needs neither: with both left zero, the module booted into CBoot's flashing server using only the prototype's five files. They matter only for a first flash.
- `rcm_*`'s CMAC must be valid, since the boot ROM checks it. The recovery applet accepted the blob's unsigned `cboot.bin` and DTB (CMAC slot all 0xEE), but signing them costs nothing.

Expected sha256 of the five files built from the pinned kit:

| file | sha256 |
| --- | --- |
| `rcm_0_encrypt.rcm` | `9eb594faa8f13d1d99368febc15fdf880e744e53716e662de86141ecc81403df` |
| `rcm_1_encrypt.rcm` | `5a299e8df8eccc5db2596f8492e7744ebae99473b2c915cbb1daa279e907c643` |
| BCT (no bootloader entries) | `f7e6362074bcb8d4408ce2fbf497ceb947b765884b16b69f919bb12fbb84e0ea` |
| `cboot.bin` | `9a0a80e58da44538d35a7aa15334f6e8ba9eae97b4a37899de7716a437194f22` |
| DTB | `af297f9c8e191dff7f50dcd686f3d8191f9d7fa00b8386c0ba25fea79fffc075` |

## Todo

- [x] `internal/l4t`: fetch and verify the kit, streaming, and extract the four inputs.
- [x] Build the five boot-chain files, with CMAC from RFC 4493.
- [x] Tests: RFC 4493 vectors, synthetic inputs, and a test gated on a local copy of the kit that checks the hashes above.
