---
# gosd-gr8j
title: 'jetson-flash: flash any OS that boots from NVIDIA''s stock chain, not just gosd images'
status: completed
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

## Open questions, after the bench

- **Write time: settled.** `APP` writes at about 10 MB/s (7 GiB in about 12 minutes), so a full 14 GiB filesystem takes about 25. Sparse writes stay unexplored: a smaller image grown once it's running does the same job.
- **Growing the filesystem: settled.** JetPack's `nvresizefs` only grows `APP` when it's the disk's last partition, and in NVIDIA's eMMC layout it's the first, so JetPack never grows it by itself. `sudo resize2fs /dev/mmcblk0p1` grows it online to the whole 14 GiB.
- **Kernel and device tree.** Anything other than JetPack needs a kernel that runs on Tegra210 with NVIDIA's device tree, or its own `FDT` line in `extlinux.conf`. Generic Ubuntu arm64 isn't built for the Nano. Only JetPack is proven.
- **Kit version.** Only JetPack 4.6.6 (R32.7.6), the boot chain's own release, is proven. An older JetPack's kernel with R32.7.6's device tree is untested.

## Out of scope

- Downloading OS images.
- Other Jetson modules, including the SD-card developer kit, whose boot chain lives in QSPI.

## Todo

- [x] Accept MBR (FAT32 or Linux partition 1), GPT (`APP` or partition 1) and bare FAT/ext4 images, refusing the rest before USB.
- [x] Generic wording in `flash --help`, its errors and the flashing guide.
- [x] Bench: JetPack's Ubuntu boots on node 3 from a `jetson-flash` flash.

## Bench run (2026-11-09, node 3)

The image was NVIDIA's R32.7.6 sample root filesystem, extracted into a loop-mounted 7 GiB ext4 image, after `apply_binaries.sh` and `l4t_create_default_user.sh`, built in an arm64 Ubuntu 18.04 container. It was flashed as a bare ext4 image by this branch's `jetson-flash` on the BMC.
- The flash took 12 min 30 s, with every partition, both flushes and the BCT, and no errors.
- U-Boot: `Found /boot/extlinux/extlinux.conf`, then `/boot/initrd` and `/boot/Image`. Linux `4.9.337-tegra` started, and the console reached `Ubuntu 18.04.6 LTS jetpack-nano ttyS0` and a login prompt about 20 s later.
- Over SSH:
  - `/etc/nv_tegra_release` says R32 REVISION 7.6;
  - `/dev/mmcblk0p1` is 14G and mounted as `/` (6.8G);
  - `sudo resize2fs /dev/mmcblk0p1` grew it to 14G online.
- **`flash.sh`'s `APP` edits aren't needed.** The image had them: NVIDIA's command line appended to `extlinux.conf`'s `APPEND`, and the DTB copied to `/boot/dtb`. U-Boot's `append:` line shows `${cbootargs}` already carries `root=/dev/mmcblk0p1 rw rootwait rootfstype=ext4 console=ttyS0,…` from `LNX`, so the append only repeats it. `extlinux.conf` has no `FDT` line, so the DTB copy isn't read. The flashing guide leaves both out.
- **Harmless warnings, from the carrier having no P3449 EEPROM:**
  - CBoot's `shim_eeprom_update_mac … Failed to update 0 MAC address`;
  - CBoot's `Warning: Find Partition via PT Failed`;
  - U-Boot's `reserving fdt memory region failed`;
  - the kernel's `tegra-i2c … i2c transfer timed out addr: 0x50`.
  The module still got an address over DHCP and answered as `jetpack-nano.local`.

## Summary of Changes

- `bootPartition` tells a GPT disk image (partition `APP`, else the first), an MBR disk image (partition 1) and a bare FAT or ext4 filesystem image apart by their own bytes. It then checks that the chosen region holds FAT or ext4, refusing everything else before any USB activity.
- `flash`'s help, its errors and the flashing guide no longer assume a gosd image. The guide gains a section on flashing another OS, with JetPack as a worked example.
