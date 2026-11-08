---
# gosd-9o0m
title: 'Jetson Nano eMMC module: bench bring-up'
status: todo
type: task
priority: normal
created_at: 2026-11-01T11:18:51Z
updated_at: 2026-11-07T22:18:22Z
parent: gosd-72je
blocked_by:
    - gosd-wxy7
    - gosd-j18s
---

Bench bring-up of the eMMC module (P3448-0002, board 3448-0002-401, revision G.0) in Turing Pi 2 node 4, with its console on the node's UART2 DEBUG header. The procedure is in `docs/jetson-nano-flashing.md`, and the findings are in `docs/development/jetson-nano.md` and `docs/development/turing-pi-2-bmc.md`.

**Proven on 2026-11-07, with `examples/hello` built from main at 6d68aa1:**

- [x] A blank module has nothing bootable on it: no console output, no recovery-mode device, no network traffic.
- [x] NVIDIA's offline massflash blob, run from the BMC, writes NVIDIA's boot chain plus GoSD's boot partition as `APP` in about a minute.
- [x] NVIDIA's U-Boot (`U-Boot 2020.04-g904285e3a7`, R32.7.6) runs distro boot: `bootdelay=2`, `boot_targets=mmc1 mmc0 usb0 nvme0 pxe dhcp`. Its environment checksum fails and it uses the default environment.
- [x] It finds `/extlinux/extlinux.conf` on our FAT32 `APP` partition (`Scanning mmc 0:1`) and loads the initramfs (10.9 MB), the `Image` and our DTB. The FAT-boot question is settled.
- [x] The mainline 6.18.37 kernel boots with our command line. `r8169` brings up the RTL8168f/8111f with the MAC CBoot supplies, and `tegra-xusb` loads the pinned `xusb.bin`.
- [x] U-Boot sees the M.2 NVMe drive (`nvme0`, 477 GB) and gets a DHCP lease.

**Blocked:**

- [x] gosd-init mounts its boot partition from `/dev/mmcblk0p1` with gosd-wxy7's patch: an HS400 eMMC, NVIDIA's 17 partitions visible.
- [ ] `/data` on the NVMe: gosd-j18s.
- [x] Network up (DHCP lease), mDNS (`hello.local`), HTTP 200 from `hello`, NTP sync. The kernel reports 3899012K/4161536K RAM available (4 GiB). `r8169` asks for `rtl_nic/rtl8168f-2.fw`, which isn't shipped; the link works without it.
- [ ] Power-cycle survival; a USB stick visible to the `disk` package.
