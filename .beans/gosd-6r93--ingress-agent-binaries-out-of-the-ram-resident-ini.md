---
# gosd-6r93
title: 'Ingress agent binaries out of the RAM-resident initramfs: exec from GOSD-BOOT only when configured'
status: todo
type: task
created_at: 2026-08-08T08:09:29Z
updated_at: 2026-08-08T08:09:29Z
---

JP question at the 2026-08-08 bench: unconfigured ingress agents start no process (verified — resolveMode returns after one log line, before any StartProcess), but the BINARIES still cost RAM unconditionally because the rootfs is the RAM-resident initramfs: ~25MB cloudflared + ~16MB gosd-tsfunnel ≈ 41MB pinned on any --ingress image, configured or not.

Proposal: place ingress agent binaries on the GOSD-BOOT FAT partition instead of inside the initramfs, and have gosd-init exec them from the mounted /boot only when the section is configured. Unconfigured device → zero bytes of agent RAM; configured device → file-backed (evictable) pages instead of pinned initramfs bytes. Boot-partition SIZE is unchanged (the initramfs lives there today anyway).

Design points to settle:
- Amends the 'nothing executable loose on a partition' convention — record the carve-out (gosd-shipped agent binaries only; identity-covered like all boot files; staticelf-verified at build as today).
- /boot mount flags: exec must be permitted for these files (check current vfat mount opts in boot/mounts.go); FAT has no x-bits — fmask decides.
- BinaryPath in the runtime modules moves from /bin/<agent> to /boot/<name>; the modules' baked-flag contract (config.json bit, never probing the fs) is UNCHANGED.
- Crash-safety: read-only /boot, no write path — no new crash-ordering surface.
- Measure before/after RSS on a real board (bench) to confirm the win; the win must hold on the 512MB pi-zero-2w especially.
- Interaction with --with-external stays untouched (user externals remain initramfs-resident, app-owned).

Not urgent — both agents are opt-in per image — but it changes 'is it safe to always bake ingress into shipped images' from 41MB-of-RAM to free, which is a meaningfully different default posture.
