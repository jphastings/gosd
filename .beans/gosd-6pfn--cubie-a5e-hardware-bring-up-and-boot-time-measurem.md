---
# gosd-6pfn
title: 'Cubie A5E: hardware bring-up and boot-time measurement'
status: in-progress
type: task
priority: deferred
created_at: 2026-08-06T22:34:12Z
updated_at: 2026-08-16T21:37:58Z
parent: gosd-h1wv
blocked_by:
    - gosd-zh95
---

Bench-verify a gosd-built image on JP's Cubie A5E: flash via sdwire, serial console capture, boot to /app, Ethernet (EMAC0) DHCP + mDNS, /data adoption + dataexpand, gosd.toml provisioning, USB gadget if the research bean found it viable, exFAT/disk if applicable. Record a power-on→/app boot-time baseline (later optimization gets its own bean, per fleet convention).

Watch for the known Allwinner-specific risks from the research bean: PMIC regulator dependencies for the SD rail (a kernel missing the AXP drivers may lose the card mid-boot), and BootROM offset mistakes (no SPL banner on serial = wrong offset).

## Todos

- [ ] First boot: SPL/U-Boot banner → extlinux → kernel → gosd-init → /app on serial
- [x] Ethernet: link, DHCP, mDNS answer
- [ ] Data partition: adoption gate, dataexpand, reboot persistence
- [x] Provisioning: config-tree hand-edit honored (config/hostname; gosd.toml is pre-gosd-rw6n wording)
- [ ] Boot-time baseline recorded here + COMPATIBILITY.md footnotes updated with hardware-verified status
- [x] File follow-up beans for anything found (field-report pattern)

DEFERRED (JP, 2026-08-07): the Cubie A5E is still in the post — bring-up starts when the board physically arrives and goes on the sdwire rig. Software side is fully activated (artifacts v0.9.0, public board, PR #205), so this is hardware-gated only.


## Bench session 2026-08-16 (first hardware on the rig)

Board: Radxa Cubie A5E, **1GB LPDDR4x variant**, on the sdwire rig, CP2102N on
ttyS0 @ 115200. Image: `examples/hello`, `gosd build ./examples/hello
--board cubie-a5e --data-size expand`, stock artifacts v0.10.0.

**Result: boot BLOCKED in U-Boot SPL — DRAM init fails before the kernel.**
Root cause is upstream mainline's single fixed set of vendor DRAM parameters,
which don't suit this board's 1GB-variant DRAM chip; full evidence, decode of
the failure address and the decision it forces are in bean gosd-84b8.

### Verified on hardware anyway (each proven by the failing boot itself)

- **BootROM finds our SPL at byte 8192** — the epic's central new-to-gosd
  assumption (one raw write of `u-boot-sunxi-with-spl.bin`, where Rockchip
  needs two) is correct on real silicon.
- **The SPL that runs is ours** — banner build date matches the v0.10.0
  cubie-a5e artifact, so the U-Boot/TF-A Docker pipeline, FIT assembly,
  artifact release, download+sha256 verification and image assembly are all
  sound end to end.
- **Console is ttyS0 @ 115200 and reads cleanly** on the CP2102N — the
  research bean's call was right, and none of the Rockchip 1.5M baud garble
  applies here.
- **Boot chain reaches SPL in well under a second** from mains-on.
- `gosd build` image structure verified by reading the built image back:
  MBR partition 1 = FAT32 LBA at 16MiB, 256MiB; `eGON.BT0` header at 8192
  carrying `allwinner/sun55i-a52...`; boot partition holds Image, board DTB,
  initramfs.cpio.zst, `extlinux/extlinux.conf` (correct `console=`, `fdt`,
  `gosd.board=cubie-a5e`) and the full `config/` tree with `.explain.md`
  sidecars.
- Bench flow itself is healthy: `sdwire flash` wrote and handed the card over,
  `sdwire power cycle` cycles the board (config resolves, power plugin
  reports controlling device "bench").

### Not yet verifiable (all gated behind DRAM)

Kernel boot, gosd-init, /app, Ethernet DHCP + mDNS, /data adoption +
dataexpand, config-tree provisioning, USB gadget, boot-time baseline.

### Note on this bean's own text

The todo below says "gosd.toml hand-edit honored" — that predates epic
gosd-rw6n, which replaced the single boot file with the per-attribute
`config/` tree. The provisioning check to run once the board boots is a
config-tree edit (e.g. `config/hostname`), not a gosd.toml one.


## Results with the DRAM blocker worked around (same session)

To get past gosd-84b8 and actually exercise the board, U-Boot was rebuilt from
the SAME pinned sources with only the community's 1GB-variant DRAM parameters
added as a config fragment (`TPR6/TPR10/TPR11/TPR12`), and images were built
with `--artifacts-dir`. **That one change took the board from "halts in SPL" to
a full clean boot**, first try:

```
U-Boot SPL 2026.04
DRAM: 1024 MiB
NOTICE:  BL31: Detected Allwinner A523 SoC (1890)
U-Boot 2026.04 ... Model: Radxa Cubie A5E
Found /extlinux/extlinux.conf ... Starting kernel ...
[gosd] started /app (pid 150)
```

Everything below was verified on that basis. The stock artifact still cannot
boot this board — see gosd-84b8 for the decision that needs making.

### Verified

- **Full boot chain**: SPL → BL31 (TF-A fork pin works) → U-Boot proper →
  extlinux → kernel → gosd-init → /app.
- **Ethernet (EMAC0)**: `dwmac-sun8i` binds, carrier up, DHCP lease
  (`[gosd] eth0: lease {192.168.1.201 ...} via gateway 192.168.1.1`), U-Boot
  gets its own lease too.
- **mDNS + HTTP**: `mdns: answering as cubiebench.local on all up interfaces`;
  from the Mac, `ping cubiebench.local` → 0.36ms and `curl` served the app.
- **Data partition**: `dataexpand` created and formatted a 59.2GiB FAT32 volume
  on first boot of an `expand` image; a fixed-size image's volume was adopted
  cleanly across 8+ reboots (`.gosd-data` marker, `.gosd-boot-count` = 8, and
  app files all persisted, readable on the Mac).
- **Config-tree provisioning**: hand-editing `config/hostname` on the card was
  honoured, and the change was recorded for re-flash:
  ```
  [gosd] hostname set to "benchdiag"
  [gosd] hostname set to "cubiebench" (config/hostname applied)
  [gosd] kept for the next re-flash: config/hostname
  ```
  mDNS followed the new name. (This is the config tree from epic gosd-rw6n, not
  the gosd.toml this bean's todo predates.)
- **Console** ttyS0 @ 115200, clean, no garble at any stage.
- **NTP**: `system clock synchronized via NTP: 1970-01-02T00:00:13Z ->
  2026-08-16T19:28:47Z`.

### Boot-time baseline

**10.38s from SPL banner to /app running** (n=5 clean power cycles, spread
0.15s), phases: U-Boot 9.05s → kernel 1.25s → gosd-init→app 0.06s. Power-on to
SPL banner is under a second. Comparable to the fleet (rock-4se 9.21s,
nanopi-zero2 10.33s), and ~4.5s of it is a pointless U-Boot USB scan — bean
gosd-uj4l.

**Superseded 2026-08-17 by gosd-uj4l's fix:** with the preboot USB scan
removed, the U-Boot phase measures **4.50s** (5 clean power cycles, spread
0.03s) and total SPL→app comes in at **6.98s mean** (6.70–7.75s). That makes
this board the fastest of the fleet rather than the slowest. The totals carry
more spread than this original baseline's 0.15s, so treat the U-Boot phase as
the comparable figure — see gosd-uj4l for the full table and method.

### Follow-up beans filed

- **gosd-84b8** — the 1GB DRAM blocker (blocks this bean)
- **gosd-o34r** — FAT32-over-ext4 leaves the ext4 superblock, so a healthy
  device halts on its second boot (found here, NOT board-specific)
- **gosd-yx94** — DHCP can fail permanently when the CRNG is unseeded; this
  board has no entropy source at all
- **gosd-3id7** — U-Boot once failed to read /Image from our FAT boot partition
- **gosd-uj4l** — the 4.5s USB scan

### Explained, NOT a defect

A `FAT-fs (mmcblk0p2): error, fat_free_clusters ... Filesystem has been set
read-only` appeared on the data partition late in the session. That was the
throwaway diagnostic app rewriting a file with plain `os.WriteFile` (no
fsync/rename, i.e. deliberately violating docs/runtime.md) across five abrupt
power cuts — the documented FAT hazard. A fresh `diskfmt` FAT32 image passes
`fsck.vfat` clean and survived 200 rewrite/delete cycles under Linux with no
kernel complaint, so the formatter is not implicated.

### Not tested

USB gadget (MUSB peripheral mode): the board's USB-C carries its power on this
rig, so exercising it as a gadget needs JP to re-wire the bench first.
COMPATIBILITY.md is deliberately left untouched — what this board's row should
say depends on the gosd-84b8 decision.



## Bench fault at session end (2026-08-16, ~22:00) — rig, not software

After JP disconnected and reconnected the bench, the board stopped booting off
the card. SPL runs and DRAM inits, then:

```
Trying to boot from MMC1
mmc_load_image_raw_sector: mmc block read error
Error: -38
SPL: Unsupported Boot Device!
```

Established as a rig/card fault, not a code or image fault:

- The **known-good image** — the exact binary that had booted ~15 times an hour
  earlier — now fails **identically**. That is the skill's control experiment,
  and it points at the rig.
- Immediately beforehand, `sdwire flash` failed with `no block device appeared
  for reader within 30s`; a switch dut → switch host handover brought the card
  back, and flashing then succeeded (272MiB, twice) — so writes through the mux
  work while the board's own reads fail.
- The BootROM still loads the SPL from the card fine (SPL banner + `DRAM: 1024
  MiB` appear); it is **SPL's own MMC driver** that cannot re-read the card.
  That split — conservative BootROM read OK, driver read fails — is the
  signature of a marginal card or connection rather than bad data, and it
  rhymes with nanopi-zero2's card-specific U-Boot MMC trouble (bean gosd-0abt).

Suggested next step before any more bring-up work: reseat the card and the
board-side SD connection, and try a **different SD card** — this one has taken
half a dozen full-card writes today. Nothing here needs a code change.

### Still outstanding

- Full boot-to-/app from the repo-built U-Boot (blocked by the above; the same
  configuration is already proven, byte-identical `.config`, and reached
  `DRAM: 1024 MiB` from the committed recipe).
- USB gadget, which needs the board's USB-C re-wired off power.



## Rig fault resolved, and the committed recipe verified end to end

The `mmc block read error, Error: -38` scare was **not a failing card**: during
the reseat the SD card ended up in the CalDigit dock's own SD slot
(`TS4 Card Reader`) rather than the SDWire's (`USB3.0-CRW`, serial
20120501030900000). macOS happily showed the card as an external disk the whole
time, which is what made it look like a healthy rig with a dying card, while
`sdwire disk` correctly reported no block device. Diagnostic that settles it in
one step: map each `/dev/diskN` to its `Device / Media Name` — the mux's reader
is `USB3.0-CRW`, and anything else is the wrong slot.

With the card back in the mux, the image built from the **committed** recipe
(`dram-1gb.config`, PR #292) booted first time:

```
U-Boot SPL 2026.04 (Aug 16 2026 - 20:36:24 +0000)
DRAM: 1024 MiB
...
[gosd] data partition already present on /dev/mmcblk0p2
[gosd] started /app (pid 147)
gosd hello, host=hello board=cubie-a5e boots=2
```

`boots=2` and `data partition already present` also re-confirm data adoption
across a reboot. That closes the only verification gap the PR was carrying.

Networking on this particular boot did NOT come up — a second, independent
reproduction of the DHCP-vs-unseeded-CRNG bug (bean gosd-yx94, updated with the
detail). Unrelated to the DRAM change; it predates it and reproduces on stock
images.
