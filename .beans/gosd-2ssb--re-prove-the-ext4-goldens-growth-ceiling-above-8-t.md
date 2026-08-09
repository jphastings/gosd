---
# gosd-2ssb
title: Re-prove the ext4 golden's growth ceiling above 8 TiB in CI
status: todo
type: task
created_at: 2026-08-09T09:54:59Z
updated_at: 2026-08-09T09:54:59Z
---

internal/diskfmt/ext4golden/manifest.json records verifiedGrowthCeilingBytes = 8 TiB, but that is a BUILD-HOST limitation, not a property of the golden image: build.sh tries 16 TiB and falls back until the host's own filesystem can represent a file that large, and colima's default VM root is a non-64bit ext4 that tops out just under 16 TiB. The golden ships meta_bg precisely to escape resize_inode's ~8 TiB reserved-GDT cap, and kernel.org documents meta_bg to 2^32 groups / 512 PiB.

The gap is that the documented ceiling is far above the empirically proven one, which invites exactly the wrong conclusion (JP read the 8 TiB as a real limit and proposed capping --data-size=expand at it, 2026-08-09; see gosd-95yu's discussion). Re-run the verification somewhere whose own filesystem is not itself capped — the README names qemu-virt's CI environment as a better fit than a maintainer's Docker Desktop/colima VM — and update manifest.json's verifiedGrowthCeilingBytes plus the README's ceiling paragraph with what was actually proven.

Not urgent: dataexpand only ever grows the boot device (SD card or eMMC), and nothing purchasable comes near 8 TiB. This is about the recorded fact being misleading, not about a reachable bug.
