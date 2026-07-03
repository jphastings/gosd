---
# gosd-kkz4
title: 'gosd-init core: mounts, app supervision, console logging'
status: completed
type: task
priority: normal
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-03T08:17:39Z
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

- [x] Implement with unit tests for config parsing, cmdline parsing, backoff logic (pure functions)
- [x] Zombie reaping correct even for double-forked grandchildren
- [x] Handle SIGTERM/SIGINT as no-ops (PID 1 must not die)

## Acceptance
Boots to a running supervised /app on hardware (validated by board bring-up tasks); `go test ./cmd/gosd-init/...` passes on macOS and Linux.

## Summary of Changes

Implemented gosd-init's core boot sequence in `cmd/gosd-init`, replacing the placeholder:

- `cmd/gosd-init/internal/initcfg`: pure, build-tag-free package owning the config.json schema (`Config{Board,Hostname,Wifi{SSID,Passphrase}}`) and gosd.board/gosd.debug kernel cmdline parsing. 100% test coverage. Exported so later beans (gosd.toml, provisioning parser, networking, WiFi, time sync) can import it.
- `cmd/gosd-init/internal/boot`: orchestration package with no build tags — `Run()` (the 8-step sequence), `Supervisor` (restart/backoff loop), `Backoff`, `MountBootPartition` (retry logic), and `Logger` are all pure given injected interfaces (`Mounter`, `HostnameSetter`, `AppStarter`, `Reaper`, `Rebooter`), so they're unit-tested with fakes on macOS (~90% coverage). Real Linux syscall implementations (mount, sethostname, wait4-based PID 1 zombie reaping, reboot, /dev/console, SIGTERM/SIGINT ignoring) live in `platform_linux.go`; `platform_other.go` provides stub implementations so the package still builds on macOS.
- `cmd/gosd-init/main.go`: thin entrypoint wiring `Deps`/`Options` and calling `boot.Run`.
- Added `golang.org/x/sys` as a direct dependency (pinned v0.46.0) for the Linux syscalls.

Zombie reaping (`linuxReaper`) uses a single SIGCHLD-driven `wait4(-1, WNOHANG)` loop so it reaps /app and any double-forked grandchildren reparented to PID 1 through the same path, avoiding the classic races between `os/exec`'s own reaping and a PID-1-wide reaper.

An initial version read `/proc/cmdline` in `main.go` before the boot sequence's early mounts had run — since `/proc` isn't mounted until step 1, this would have silently disabled `gosd.board`/`gosd.debug` on real hardware. Fixed by moving both config.json and cmdline reading into `boot.Run` itself (via injected `Deps.ReadConfig`/`Deps.ReadCmdline`), called at the locked step-3 point, after `mountEarly` succeeds. A regression test (`TestRunReadsCmdlineOnlyAfterProcIsMounted`) guards this ordering.

Verified: `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), and `go test ./...` all pass on macOS; `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/gosd-init` produces a static ELF binary; `go vet` also passes cross-compiled for linux/arm64.

### Deviations / open questions for hardware bring-up

- The exponential backoff resets to its base delay after /app has run for 30s continuously ("stable"). The bean doesn't specify a reset condition, only "exponential backoff capped at 10s" — this is my interpretive addition (matches systemd/Kubernetes-style crash-loop backoff) so a device that crash-loops once early doesn't stay slow to restart for its whole uptime. Flagging in case a different reset policy (or none) is wanted.
- `linuxReaper` narrows, but cannot fully eliminate, a race between `cmd.Start()` returning and the reaper's `expect(pid)` call: if /app exits in that narrow window, its exit could in theory still be reaped and discarded as an "unrelated" pid before `expect` registers it. This only matters for a child that exits within microseconds of being forked, and can't be tested from macOS since the affected code is Linux-only — worth a specific stress-test on hardware bring-up.
- Mount flags (`MS_NOSUID`/`MS_NODEV`/`MS_NOEXEC`) for the four early mounts are my best-practice defaults (matching common distro choices), not spelled out in the bean; worth confirming they don't conflict with anything board-specific during bring-up.
