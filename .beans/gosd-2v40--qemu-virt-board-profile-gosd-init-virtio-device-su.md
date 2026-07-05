---
# gosd-2v40
title: qemu-virt board profile + gosd-init virtio device support
status: todo
type: task
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T07:07:13Z
parent: gosd-c54j
blocked_by:
    - gosd-5wm0
---

Register board id qemu-virt (internal: EXCLUDED from the default no---board build set and from `gosd build` help examples — add an Internal marker to the boards registry). Artifacts: Image only. BootFiles: Image + initramfs.cpio.zst at the FAT root (no config.txt/extlinux — qemu boots -kernel/-initrd directly; the FAT partition still carries gosd.toml and receives cloud-init files, keeping provisioning testable). RawWrites: none. FirmwareFiles: empty.

gosd-init: add /dev/vda1 (boot) and /dev/vda2 (data) to the device-candidate lists alongside mmcblk — behind the same probe logic; no qemu-specific code paths.

- [ ] Board profile + integration test (fake artifacts; assert default build does NOT include qemu-virt, explicit --board=qemu-virt does)
- [ ] gosd-init candidate lists + tests
- [ ] Update CLAUDE.md board IDs line to mention qemu-virt (internal)
