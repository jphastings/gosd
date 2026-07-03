---
# gosd-kkz4
title: 'gosd-init core: mounts, app supervision, console logging'
status: todo
type: task
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-02T21:03:54Z
parent: gosd-ko20
---

The PID 1 skeleton in `cmd/gosd-init`. Static pure-Go binary, runs as /init from the initramfs. Linux-only code paths behind build tags so `go test` still runs on macOS for the pure logic.

Boot sequence (locked):
1. Mount devtmpfs on /dev, proc on /proc, sysfs on /sys, tmpfs on /run (all with sensible flags; MS_NOSUID etc.)
2. Open /dev/console for logging; every log line prefixed `[gosd] `
3. Read /etc/gosd/config.json (schema owned here: board, hostname, wifi{ssid,passphrase}) and kernel cmdline params (gosd.board, gosd.debug)
4. Set hostname (sethostname syscall)
5. Mount the GOSD-BOOT FAT partition read-only at /boot: try /dev/mmcblk0p1 then /dev/mmcblk1p1 (no udev; retry for up to 10s — the MMC controller may still be probing)
6. Start /app as a child with env GOSD_BOARD, GOSD_HOSTNAME, stdout/stderr to console; do NOT block on network
7. Supervise: restart /app on exit with exponential backoff capped at 10s; reap all zombies via SIGCHLD + wait4 loop (PID 1 duty)
8. If gosd-init itself hits a fatal error: log, sync(2), sleep 5s, reboot(2)

- [ ] Implement with unit tests for config parsing, cmdline parsing, backoff logic (pure functions)
- [ ] Zombie reaping correct even for double-forked grandchildren
- [ ] Handle SIGTERM/SIGINT as no-ops (PID 1 must not die)

## Acceptance
Boots to a running supervised /app on hardware (validated by board bring-up tasks); `go test ./cmd/gosd-init/...` passes on macOS and Linux.
