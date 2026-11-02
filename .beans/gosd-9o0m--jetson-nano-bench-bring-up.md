---
# gosd-9o0m
title: 'Jetson Nano: bench bring-up'
status: todo
type: task
priority: normal
created_at: 2026-11-01T11:18:51Z
updated_at: 2026-11-01T11:53:20Z
parent: gosd-72je
blocked_by:
    - gosd-cxio
---

Flash the internal-only image to a P3450 dev kit whose QSPI is on R32.6.1+ (JetPack 4.6; check the version partition or just boot NVIDIA's JetPack 4.6 SD image once) and walk the standard bring-up checklist: build → flash → boot → serial console → network up → mDNS + HTTP → power-cycle survival. Interrupt U-Boot and run `printenv kernel_addr_r ramdisk_addr_r fdt_addr_r board soc fdtfile` first: the 14 MiB initramfs ceiling was derived from R32.6.1 source, and a saved environment overrides the defaults. Then prove NVIDIA's U-Boot finds extlinux.conf on our MBR/FAT32 partition 1. Capture the full U-Boot log, including any fdt_copy_node_paths / "phandle mismatch" lines. Also check: USB host (xusb.bin loads; a USB stick appears for the disk package); Ethernet, and whether r8169 asks for an rtl_nic patch; the RAM size the kernel reports; USB gadget on the micro-USB port, if the profile claims it. Record surprises in docs/development/jetson-nano.md.
