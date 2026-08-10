---
# gosd-ssth
title: Decide whether the Pi family kernels should gain CONFIG_EXT4_FS
status: completed
type: task
priority: normal
created_at: 2026-08-09T09:35:37Z
updated_at: 2026-08-10T10:14:46Z
---

gosd-95yu makes ext4 GOSD-DATA opt-in but refuses it at build time for pi-zero-w, pi-zero-2w and pi-3b, whose stock kernels do not build CONFIG_EXT4_FS. Enabling it would let those boards use a crash-resilient /data too, but it is a family-wide raspberrypi/linux commit-pin change plus the full artifacts release dance, and kernel size on pi-zero-w (armv6, smallest board) is the sensitive constraint — measure the initramfs/kernel growth before committing. JP's call; left as an open question on gosd-95yu rather than assumed.



## Answered on the bench (2026-08-09): yes, and the kernel work was already done

This bean's premise is out of date. The Pi kernels ALREADY build CONFIG_EXT4_FS — bean gosd-19kw added it to all three fragments and it shipped in artifacts/v0.10.0, which internal/artifacts.Version already pins. gosd-95yu's build-time refusal was written against the committed kernel.config snapshots (stale last-real-build files, unchanged since 22bda8a), not the fragments or the release. COMPATIBILITY.md contradicts itself as a result: 'ext4 on attached disks' is ✅ for the Pis, 'ext4 data partition' is ❌, over the same kernel.

Verified against the released artifacts, not the beans: all three v0.10.0 kernel.configs carry CONFIG_EXT4_FS=y (+JBD2/CRC16/FS_IOMAP/BUFFER_HEAD), and all three compiled kernels contain the driver including ext4_resize_fs. pi-zero-w needed its gzipped zImage decompressed first (gzip magic at offset 28704) — a naive strings(1) on it finds nothing and looks like a false negative; once unpacked its 6.18.37 32-bit kernel has the meta_bg resize paths (add_new_gdb_meta_bg, ext4_convert_meta_bg) the golden's ^resize_inode,meta_bg layout needs.

Kernel-size worry (this bean's stated deciding constraint) is already paid and measured, v0.9.0 → v0.10.0: pi-zero-w +303 KiB, pi-zero-2w/pi-3b +860 KiB each, against a 256MiB default boot partition. Non-issue, and a sunk cost either way.

HARDWARE PROOF (pi-zero-2w on the bench, --data-filesystem=ext4 --data-size=expand, examples/hello): the flashed image contains ONLY partition 1 (272MiB, boot). After boot the 15.9GB card reads p1 hello-boot FAT32 + p2 Linux 15.6GB. In dataexpand's expand path WriteMBR runs LAST — only after FormatEXT4 → SyncDevice → EstablishEXT4 → SyncDevice all return clean, and establishEXT4 itself mounts the filesystem, grows it via EXT4_IOC_RESIZE_FS, writes+fsyncs the marker into it, then unmounts. So that partition entry existing is proof the Pi loaded the ext4 driver, mounted ext4 read-write, grew the 512MiB golden to fill the card, and wrote a file into it. A reflash (image with no p2) rebuilt p2 at full size again, so the reflash-upgrade path completes too.

NOT yet proven on hardware: adopt-vs-reformat after reflash (indistinguishable from the MBR alone), and the boots=N power-cut durability/journal-replay assertion. Both need to read /data, which needs either working serial or the app reachable over the network.

BENCH BLOCKER: the Pi Zero 2W's serial console is silent — a single clean tio reader at 115200, zero bytes across boots we know happened. Not the image (enable_uart=1, console=serial0,115200 verified on the card itself) and not the board (it demonstrably boots and does real work). The Pi TX → adapter RX / GND wiring needs a physical check. Two traps found en route, both now in memory: sdwire MUST be driven as '-s bench' (the port-suffixed identity resolves the mux but silently drops the power: config, so every power cycle is a no-op that exits 0), and a separate stty process does not stick on macOS (port reverts to 9600; use tio with stdin held open).

Recommendation: this is no longer a 'decide whether to change the kernel pin' bean. The remaining work is flipping EXT4Support() on the three Pi boards, correcting blockmount.remedyFor's now-false runtime error text, COMPATIBILITY.md and CLAUDE.md, and reworking the tests that use a Pi as the ext4-incapable fixture.



## Verification complete (2026-08-10)

The question this bean asks is answered YES, and the kernel change it contemplates was already shipped. Full hardware results — first-boot format/grow/mount, re-adoption across reflashes (boots=10, never reset), and crash durability with a power cut 5.7s after the write (boots=12 -> 13) — are recorded in bean gosd-7bwv. Nothing here needs a kernel pin change or an artifacts release.

Remaining work is the code/doc cleanup only: flip EXT4Support() on pizero2w/pizerow/pi3b, correct internal/blockmount.remedyFor's now-false runtime error text, update COMPATIBILITY.md (the ext4-data row + [^pi-data-ext4] footnote, which currently contradicts the [^pi-ext4] row above it over the same kernel) and CLAUDE.md's locked decision, and rework the tests that use a Pi board as the ext4-incapable fixture — with every public board ext4-capable there is no real board left to play that part, so those need a fake board. Note pi-zero-w and pi-3b are flipped on the same released-kernel evidence but were NOT themselves bench-booted; only pi-zero-2w was.
