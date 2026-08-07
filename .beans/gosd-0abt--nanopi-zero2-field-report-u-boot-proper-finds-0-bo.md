---
# gosd-0abt
title: 'NanoPi Zero2 field report: U-Boot proper finds 0 bootflows on SD that SPL booted (atfs bench)'
status: todo
type: bug
priority: normal
created_at: 2026-08-06T19:34:39Z
updated_at: 2026-08-06T21:34:17Z
---

Field bug report from the atfs bring-up (atfs bean ATFS-kzvr; report file at ~/src/personal/atfs/gosd-bugreport-nanopi-zero2-no-bootflows.md). NanoPi Zero2 with eMMC fitted, image built by gosd @ 6f8d7db (2026-08-03) with `--board nanopi-zero2 --data-size expand --hostname atfs --placeholder network-config=4KiB`, artifacts v0.8.0: full SPL→BL31→U-Boot chain runs from SD, then bootstd scans both MMC bootdevs silently, `(0 bootflows, 0 valid)`, kernel never loads.

## Desk triage (2026-08-06) — three of four hypotheses ruled out

**1. Artifacts drift since the gosd-odp7 validation: RULED OUT.** Released v0.8.0 nanopi-zero2 U-Boot proper is byte-identical to the hardware-validated v0.6.0 build except 6 bytes — the "Jul 24 2026 - 00:26:25" → "Jul 26 2026 - 10:58:58" version-string timestamp. FIT control DTB (fdt-1) and all three BL31 sections: identical sha256. Runtime rk3528-nanopi-zero2.dtb and kernel.config: byte-identical across releases. idbloader diff is 70 bytes (SPL timestamp; SPL provably worked in the failing log). `git diff artifacts/v0.6.0 artifacts/v0.8.0 -- build/boards/nanopi-zero2` is empty; rkbin pins unchanged (DDR v1.13 / BL31 v1.21 match the failing log).

**2. Image/CLI regression (incl. the emmc/diskfmt work): RULED OUT for this failure stage.** The emmc/blockmount/diskfmt runtime work is kernel-stage code — U-Boot never sees it. CLI-side image changes since validation (gosd-e3e3 FAT trim — documented+tested no-op at the 256MiB default; gosd-m70t --boot-size; gosd-49it placeholders) were checked by building a byte-exact-flags reproduction image with the reporter's CLI code and walking its FAT with U-Boot fat.c semantics (strict 0x00-terminator directory walk, FAT32 BPB checks, cluster-chain following): clean. Valid self-consistent FAT32, /extlinux/extlinux.conf reachable, content byte-identical to the reporter's own dissection. Note the qemu CI job boots with -kernel directly, so it does NOT regression-test U-Boot's FAT/extlinux parsing — the walk above was done manually.

**3. "eMMC possibly not fitted during validation": FACTUALLY WRONG.** gosd-odp7 refitted the eMMC on 2026-07-24 and captured 6 power-cycle boots including the eMMC-refit boot, every one through U-Boot's bootflow scan to the app, on the functionally-identical binary. (Bench eMMC's p1 was invalid FAT — see gosd-pcwl; an eMMC with different content still cannot affect the SD bootdev scan, which runs first and found nothing.)

**4. Remaining suspect: the reporter's hardware combination.** Most likely their specific 16GB ex-Armbian SD card failing U-Boot proper's per-DT clock/mode renegotiation while SPL's conservative init reads it fine (their hypothesis 3); an electrical interaction from their particular eMMC module can't be fully excluded from the desk. bootstd autoboot swallows per-bootdev errors, which is why the scan is silent.

## Update 2026-08-06 22:xx — hello image reproduces on the bench; root-cause hypothesis narrowed to U-Boot UHS negotiation

JP flashed a freshly-built hello image (current HEAD, v0.8.0 artifacts, default flags) and got the IDENTICAL failure (tio log in repo root, 22:00:53): SPL boots from SD, proper-stage scan of mmc@ffc30000 silent, 0 bootflows. This eliminates everything atfs-specific. Desk analysis then closed the remaining image-side gap entirely:

- A hello image built with the CLI @ db9c17e (Jul 24, v0.6.0 pin — the validated era) is STRUCTURALLY IDENTICAL to today's everywhere U-Boot reads: MBR, BPB field-for-field, FSInfo, FATs, every directory entry, LFN chains, checksums (all validated). Sole difference: 'panic=10' in extlinux.conf's append line, which U-Boot only passes to the kernel. go-diskfs never bumped (v1.9.3 throughout).
- So bytes that provably booted this board on 2026-07-24 are equivalent to the bytes that fail today → the discriminating variable is physical: the CARD (or card-path).

**Mechanism found in the U-Boot control DTB** (extracted from the released FIT): the sdmmc node carries `sd-uhs-sdr104`, `vqmmc-supply`, `max-frequency = 150MHz`, while SPL uses `u-boot,spl-fifo-mode` conservative reads. The binary contains the full UHS machinery (mode ladder incl. UHS_SDR104, CMD11 1.8V voltage switch, tuning). On a UHS-capable card, U-Boot proper attempts SDR104 + the 1.8V switch; mainline U-Boot's dw_mshc path has no SD tuning on this controller, and a failed/one-way CMD11 switch without a workable card power-cycle aborts mmc_init entirely → bootstd swallows the error → silent scan, 0 bootflows. Non-UHS cards (plausibly the odp7 bench card) never enter this path → plain HS50 → boots. 'Worked under Armbian' does NOT clear a card: vendor U-Boots don't attempt SDR104.

**Reframe:** not a regression — no gosd change since Jul 24 is implicated — but plausibly a REAL latent gosd compatibility gap: our stock U-Boot inherits mainline's aggressive UHS DT props with no compatibility cap, so modern UHS cards can be unbootable at the proper stage. Fix direction if confirmed: disable UHS at U-Boot proper (config fragment '# CONFIG_MMC_UHS_SUPPORT is not set', or a DTS patch dropping sd-uhs-sdr104 from sdmmc) — boot files are small, HS50 is ample; check the other Rockchip boards' sdmmc nodes for the same props; artifacts tag-first/bump-second dance applies.

**Caveat the bench must resolve:** serial is wired TX-only (memory: macos-serial-bringup-gotchas), so nobody can type at the => prompt until the adapter's TXD is attached (attach AFTER power-on — cold-boot back-powering). And confirm WHICH card tonight's hello test used (assumed: the atfs 16GB ex-Armbian card, hand-flashed — no SDWire on USB at the time).

## CONFIRMED DIAGNOSIS 2026-08-06 late — U-Boot proper misdetects the card as 30.6 MiB standard-capacity

JP attached the adapter TXD and ran `mmc dev 1; mmc info` at the stuck prompt (appended to the same tio log). Result: the SD card (SanDisk, Name 'SD032', SD version 2.0) initialises WITHOUT error into MMC-legacy 25MHz — but with **High Capacity: No, Capacity: 30.6 MiB**. A modern SDHC/SDXC card must report High Capacity: Yes; U-Boot proper's CCS/capacity detection is wrong. My earlier UHS-voltage-switch mechanism is REFUTED (init reaches no UHS path and succeeds, wrongly) — though UHS config may still be implicated via the pre-negotiation card power-cycle it triggers (see below).

The 30.6 MiB (LBA 62,668) limit explains every observation exactly:
- SPL reads (idbloader LBA 64, FIT at LBA 16384, ~9 MiB): under the limit → SPL boots fine.
- MBR (LBA 0), BPB/FATs (LBA 32768+), root directory (LBA 40,866 = 20 MiB): under the limit → the FAT probe half-works, so no error surfaces.
- /extlinux dir + extlinux.conf clusters: LBA 174,192 = 85 MiB (pushed high because the 68 MB kernel Image occupies the preceding clusters) → BEYOND the believed capacity → U-Boot's blk core rejects the read → fat_exists fails → no bootflow, silently. (0 bootflows, 0 valid.)
- macOS reads the same card correctly (the 272 MB flash succeeded through the Mac's reader) → the card is not simply tiny/counterfeit; U-Boot proper on this board misnegotiates it.

Mechanism candidates, discriminable at the prompt:
1. ACMD41/OCR busy race: CCS (OCR bit 30) is only valid once the power-up busy bit sets; a card slow to come ready — especially right after U-Boot's own vmmc power-cycle of the card, which CONFIG_MMC_UHS_SUPPORT triggers before renegotiation — read too early yields CCS=0 → SC mode → CSDv2-parsed-as-v1 garbage capacity.
2. Marginal CMD line during init: R3 (OCR) responses carry NO CRC, so a dropped CCS bit is undetectable.
3. Card-specific protocol quirk tolerated by lenient hosts (kernel re-inits robustly; Armbian history proves nothing about U-Boot).

## Prompt-session results (2026-08-06, same tio log): misdetection is STICKY; eMMC perfect; card implicated

- `mmc rescan` + `mmc info` x2: SD still High Capacity: No / 30.6 MiB every time — deterministic per-init, NOT a self-healing power-up race.
- `mmc reg` not compiled into our U-Boot (CONFIG_CMD_MMC_REG absent) — no raw OCR/CSD available. Follow-up: enable it in the config fragments for future forensics.
- `mmc dev 0; mmc info`: the fitted eMMC reads perfectly — MMC 5.1, HS200 @ 200MHz, 57.6 GiB, High Capacity: Yes. Proper-stage MMC stack on the board is healthy; fault corners onto this specific SD card's negotiation with U-Boot's dw_mshc init (card answers macOS's reader correctly, so 'broken/incompatible card', not 'tiny counterfeit').
- JP flashing hello to a different card to confirm.

## Bench discriminators (priority order)

- [x] Interrupt autoboot (serial TX connected), run `bootflow scan -l`, `mmc info`, `mmc dev 1; mmc info`, `part list mmc 1` — `-l` prints the per-bootdev errors autoboot swallows and should name the failure outright (DONE 2026-08-06: mmc info named it — High Capacity: No, 30.6 MiB)
- [x] Flash the same image to a different SD card (ideally the odp7 bench card) — if it boots, it's the card (DONE 2026-08-06: different card boots hello fully — bootflow found in 213ms, kernel, gosd-init, app; HTTP verified from the Mac over IPv6)
- [x] Remove the eMMC module and retry the failing card — if that fixes it, it's an eMMC electrical/probe interaction; reproduce on the bench rig (MOOT: eMMC exonerated directly — mmc dev 0 reads perfectly at HS200/57.6GiB, and the good card boots with the eMMC still fitted)
- [ ] Fold the outcome back to the atfs reporter and into COMPATIBILITY.md if a card-compatibility caveat emerges

Related: [[gosd-odp7]] (bring-up + eMMC-refit validation), [[gosd-pcwl]] (kernel-stage eMMC shadowing, different stage), [[gosd-vzk2]] (boot-device disambiguation follow-up). Possible follow-up worth its own bean if bench confirms card-mode negotiation: consider a U-Boot fragment capping sdmmc to high-speed (no UHS) like other distros do for maximum card compatibility. (Superseded — see Resolution: UHS was not the mechanism.)

## Resolution (2026-08-06 night)

**Root cause: the specific SD card, not gosd.** A replacement card boots the identical hello image on the same board, eMMC fitted, end to end: bootflow found in 213ms, kernel → gosd-init → app, DHCP + mDNS + NTP, HTTP answered from the Mac over IPv6. The failing card (SanDisk-labelled 'SD032') deterministically misnegotiates capacity with U-Boot's dw_mshc init (High Capacity: No, 30.6 MiB, sticky across rescans) while reading fine in the Mac's USB reader; with a 30.6 MiB believed capacity, the /extlinux clusters at 85 MiB are unreadable and bootstd reports 0 bootflows with no error. SPL boots because all its reads sit below 30 MiB — the exact 'SPL works, proper fails' signature the field report described.

Forensic pointer for any future recurrence: 30.6 MiB back-computes to a SELF-CONSISTENT CSD v1 identity (~979 x 64 x 512B blocks, plausibly C_SIZE=978/C_SIZE_MULT=4/READ_BL_LEN=9) — i.e. on the proper-stage re-init the card presented a coherent 'I am a 30 MiB SDSC card' answer (CCS=0 AND a matching v1 CSD), not a corrupted parse of its real SDHC identity. Meanwhile SPL, moments earlier in the SAME power-on, necessarily got the true SDHC identity (it block-addressed the FIT at 8 MiB correctly). Same card, same power-on, two different self-consistent identities across two inits, sticky across rescans = card firmware dropping into a degraded fallback identity on re-initialisation (U-Boot proper re-inits, and with UHS compiled in may power-cycle the card mid-boot). That is out-of-spec card behaviour, unverifiable further without CONFIG_CMD_MMC_REG (raw CSD/OCR).

Every gosd layer was verified clean along the way: released v0.8.0 U-Boot = validated v0.6.0 except 6 build-timestamp bytes (control DTB/BL31 hash-identical); today's image structurally identical to a Jul-24-era build everywhere U-Boot reads (sole diff: panic=10 in the append line); FAT strict-valid incl. LFN checksums; go-diskfs never bumped; the emmc/diskfmt runtime work is kernel-stage and was never in the failure path.

Deferred follow-ups (offer beans): (1) enable CONFIG_CMD_MMC_REG in every board's U-Boot fragment for raw OCR/CSD forensics; (2) docs/COMPATIBILITY troubleshooting entry: 'SPL boots but (0 bootflows) → interrupt autoboot, run mmc info, check Capacity/High Capacity — bootstd swallows the underlying error'; (3) optional upstream-U-Boot investigation of the capacity misnegotiation with a corpus card, prepared locally per the no-third-party-PR rule.
