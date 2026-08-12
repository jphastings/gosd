---
# gosd-fuxs
title: 'Full-codebase review sweep: file beans for defects and gaps found'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:38:02Z
updated_at: 2026-07-31T08:00:05Z
---

JP asked for a review of the entire codebase, filing one bean per issue worth
addressing; JP will triage the resulting beans and commit the keepers.

Method: six parallel area reviews (build pipeline; gosd-init runtime;
storage/format correctness; public API gadget+sound; kernel/externals/CI
infra; cross-cutting quality incl. examples and docs drift), each verifying
claims against the code before reporting; findings then adversarially
triaged here before a bean is filed. Locked decisions in CLAUDE.md are not
findings; issues already tracked in existing beans are not re-filed.

## Todos

- [x] Area reviews complete (6/6: build pipeline, gosd-init runtime, storage/format, gadget+sound, kernel/CI infra, cross-cutting)
- [x] Findings triaged (verified real, deduped against existing beans)
- [x] One bean filed per confirmed finding, with evidence and suggested fix (36 beans)
- [x] Summary of Changes lists every bean filed


## Summary of Changes

36 beans filed, none committed — JP triages and commits keepers. Every
finding was verified against the code before filing (the two strongest
empirically: fsck runs against real formatter output, GOARM e_flags
experiment). Grouped by priority:

High (6): gosd-fkkr (panic bricks board), gosd-jeaw (hostname reboot
loop), gosd-1t0q (reaper race hangs supervisor), gosd-7jmj (kernel cache
key gaps; challenges locked gosd-x488), gosd-fija (idbloader/u-boot
overlap unchecked), gosd-e3e3 (FAT32 FAT under-sized at 64/128/256GiB,
fsck-proven), gosd-xq9l (trailing-space label reformats data every boot),
gosd-0r40 (gadget Apply strands configfs) — 8 total at high.

Normal (14): gosd-akk4 (network-up marker aliasing), gosd-s2yu
(resolv.conf non-atomic + DNS wipe), gosd-0esw (SNTP unbounded step,
security), gosd-vcnr (wifi backoff reset), gosd-1lx7 (stale addrs on
replug), gosd-o6tp (mdns fd leak), gosd-6i2a (dataexpand halt on
transient), gosd-83sw (audibility pass aborts), gosd-aur4 (staticelf
GOARM), gosd-7acd (patch --forward silent skip), gosd-mbdc (workflow
board-list untested), gosd-nchn (--hostname unvalidated), gosd-2maa
(RawWrites panic UX), gosd-ctkj (gadget sentinel), gosd-45bv (concurrent
emmc/disk format race), gosd-fnh8 (MassStorage mounted-device), gosd-f226
(eMMC GP partitions), gosd-zkyg (tautological exFAT checksum tests) — 18
total at normal.

Low (10): gosd-voq5 (stale board docs), gosd-9q2q (SetControl index),
gosd-vag9 (CI action pins), gosd-zyp8 (docker-context preflight),
gosd-4k5k (--data-size floor), gosd-w83z (qemu comma), gosd-htwp
(usbwebsite durable write), gosd-wo0l (hello port log), gosd-ix38 (emmc
rank divergence), gosd-8rw2 (FAT12/16 reported as FAT32).

Clean areas worth recording: fetch/artifacts (sha256, atomic rename, tar
traversal guards), catalog (schema-validated), exFAT writer itself
(spec-verified + fsck_exfat across the cluster ladder — no defect),
netlink Request discipline, platform-seam purity, dataexpand power-loss
ordering, provisioning parse robustness, container error propagation,
extbuild cache key, all examples cross-compile for both arches, zero
TODO/FIXME anywhere.
