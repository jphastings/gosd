---
# gosd-gr8j
title: 'jetson-flash: flash any OS that boots from NVIDIA''s stock chain, not just gosd images'
status: in-progress
type: feature
created_at: 2026-11-09T08:06:06Z
updated_at: 2026-11-09T08:06:06Z
---

`jetson-flash flash` is a Jetson Nano flasher, not a gosd-only tool (JP, 2026-11-09). It should flash any OS that boots from NVIDIA's stock eMMC boot chain, NVIDIA's own JetPack Ubuntu included, as well as gosd images.

## Why it's close already

What `flash` writes is NVIDIA's standard R32.7.6 boot chain, built from NVIDIA's kit:
- `LNX` is NVIDIA's own U-Boot, with NVIDIA's default command line (`root=/dev/mmcblk0p1 rw rootwait rootfstype=ext4 …`).
- `DTB` is NVIDIA's device tree for the eMMC module (P3448-0002).
- U-Boot scans `APP` (`mmc 0:1`) for `extlinux.conf`, and reads FAT or ext4.

So an OS whose root filesystem fits in `APP` and carries an `extlinux.conf` should boot from it, the same way NVIDIA's own `flash.sh` boots JetPack. The only gosd assumption is in `bootPartition`, which takes MBR partition 1 and refuses anything but FAT32 (`0x0c`), before any USB activity.

## Plan

- Accept three image shapes as `APP`'s content, told apart by their own bytes, with no flag:
  - a disk image with an MBR: partition 1, FAT32 or Linux (`0x83`), which covers gosd's images;
  - a disk image with a GPT: the partition named `APP`, otherwise partition 1, which covers NVIDIA's SD-card images;
  - a bare filesystem image (FAT or ext4, by its own magic), which covers NVIDIA's `system.img`.
- Refuse anything else, and anything bigger than `APP` (14 GiB), before USB, as now. Error messages say which shapes it takes.
- Drop the gosd wording from `flash`'s help, its errors and the flashing guide. A gosd image stays the worked example. The `--boot-size` hint stays for gosd images only.
- Bench: flash JetPack's Ubuntu to node 3 and record whether it boots to a login. The user supplies the OS image: `jetson-flash` only ever downloads NVIDIA's kit.

## Open questions

- **Write time.** `APP` writes at about 8 MB/s, so a full 14 GiB root filesystem takes about 30 minutes. NVIDIA sends a sparse image; whether CBoot's server takes one the way `flash` talks to it is unknown. A shrunk ext4 image that the OS grows on first boot avoids the question; check whether JetPack grows its root filesystem on eMMC.
- **Kernel and device tree.** Anything other than JetPack needs a kernel that runs on Tegra210 with NVIDIA's device tree, or its own `FDT` line in `extlinux.conf`. Generic Ubuntu arm64 isn't built for the Nano. This bean proves JetPack only.
- **Kit version.** The boot chain is R32.7.6 (JetPack 4.6.6). An older JetPack's kernel with R32.7.6's device tree is untested.

## Out of scope

- Downloading OS images.
- Other Jetson modules, including the SD-card developer kit, whose boot chain lives in QSPI.

## Todo

- [x] Accept MBR (FAT32 or Linux partition 1), GPT (`APP` or partition 1) and bare FAT/ext4 images, refusing the rest before USB.
- [x] Generic wording in `flash --help`, its errors and the flashing guide.
- [ ] Bench: JetPack's Ubuntu boots on node 3 from a `jetson-flash` flash.
