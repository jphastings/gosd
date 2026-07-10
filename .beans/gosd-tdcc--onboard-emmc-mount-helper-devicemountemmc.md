---
# gosd-tdcc
title: Onboard eMMC mount helper (device.MountEMMC)
status: draft
type: task
priority: normal
created_at: 2026-07-10T06:16:35Z
updated_at: 2026-07-10T07:55:56Z
parent: gosd-jge2
blocked_by:
    - gosd-899s
    - gosd-0s0m
---

App-facing helper to mount the board's onboard eMMC at a caller-chosen path,
in a new public `device/` package (the v0.3 "app-facing runtime helpers"
surface named in CLAUDE.md, not yet created).

**ON HOLD (draft), blocked by the eMMC-viability decision.** As written this
is mount-only, which is nearly useless until there's an on-device way to
*format* the eMMC — see the blocking decision bean. Do not start until that
decision lands; if it chooses a formatting path (pure-Go mkfs.vfat), re-scope
this bean to "format-if-blank + mount" rather than mount-only.

## Locked decisions (2026-07-10, may be revisited by the decision bean)
- Mount pre-formatted FAT only; no kernel/board/artifact-release changes.
- Auto-discover the onboard eMMC (no explicit device-path arg in v1).
- App-imported library only; gosd-init stays unaware of eMMC (no `GOSD_EMMC`
  env var, no `gosd.toml` auto-mount).

## Design (mount half)

Public entry point in `github.com/jphastings/gosd/device`:

    // MountEMMC mounts the board's onboard eMMC read-write at mountpoint.
    // Auto-discovers the eMMC block device (distinct from the booted SD),
    // creates mountpoint if absent, mounts its FAT filesystem
    // (nosuid,nodev + vfat "flush", mirroring GOSD-DATA). Returns ErrNoEMMC
    // when no onboard eMMC is present; an actionable formatting-hint error
    // when a device is found but has no mountable FAT filesystem.
    func MountEMMC(mountpoint string) error

- Sentinel `ErrNoEMMC` (errors.Is-checkable). Found-but-unmountable → actionable
  message per the CLAUDE.md rule (e.g. "eMMC found at /dev/mmcblk0p1 but could
  not be mounted as FAT; it must be formatted first — gosd cannot format
  on-device"). Board-agnostic: no `gosd_<board>` build tag; returns ErrNoEMMC
  where absent so it links into any board's app.

### eMMC discovery (board-independent — no mmcblk0/1 hard-coding)
1. List `/sys/block/mmcblk*`.
2. Read `/sys/block/<name>/device/type`; the Linux MMC subsystem writes `MMC`
   for eMMC and `SD` for a card. Pick the `MMC` one. (NanoPi aliases pin
   eMMC=sdhci=mmcblk0 / SD=sdmmc=mmcblk1; the sysfs `type` check is the robust
   discriminator that also covers Radxa, whose aliases live in upstream DTS.)
3. If the eMMC has partitions (child `<name>p1…` under `/sys/block/<name>/`)
   mount `p1`, else the whole-device node. Try FAT; on failure return the
   formatting-hint error.

### Package structure — mirror `boot` + `gadget`
- `device/device.go` — pkg doc + pure logic; reuse the local `msNoSuid`/
  `msNoDev` flag-constant trick from `cmd/gosd-init/internal/boot/mounts.go` so
  it compiles on macOS.
- `device/interfaces.go` — unexported `mounter` seam (same signature as
  `boot.Mounter`) + a read-only `sysfs` seam (`ReadDir`, `ReadFile`).
- `device/platform_linux.go` (`//go:build linux`) — `osMounter` wrapping
  `unix.Mount` (one-liner, copy of `boot.linuxMounter`).
- `device/platform_other.go` (`//go:build !linux`) — unsupported-platform stub
  so `go test ./...` builds on macOS. (The sysfs reader uses `os` and needs no
  build tag, like `gadget`'s `osFS`.)
- `device/device_test.go` + `device/fakes_test.go` — behavioral, fake-driven
  (`fakeSysfs`, `fakeMounter`, modelled on `boot`'s `fakes_test.go`): eMMC
  picked over SD, partition-vs-whole-device selection, ErrNoEMMC when only SD
  present, formatting-hint error when a present eMMC won't mount.

### Example + docs
- `examples/emmc/main.go` — stdlib-only, calls MountEMMC, degrades gracefully
  when eMMC absent (mirror `examples/i2cscan`). Must cross-compile arm64 AND
  `GOARCH=arm GOARM=6`.
- `COMPATIBILITY.md` — eMMC-mount row (Radxa/NanoPi supported, Pi + qemu-virt
  N/A).
- `docs/runtime.md` — eMMC storage subsection: FAT-only, must be formatted, same
  no-unix-perms / not-power-loss-robust caveats as `/data`.

## Todo
- [ ] Decision bean resolved (viability + whether to add a format path)
- [ ] `device` package: interfaces + platform_linux/other seams
- [ ] Discovery via sysfs `device/type`; partition-vs-whole selection
- [ ] `MountEMMC` + `ErrNoEMMC` + actionable formatting-hint error
- [ ] Fake-driven behavioral tests (pass on macOS)
- [ ] `examples/emmc` (cross-compiles arm64 + armv6, degrades gracefully)
- [ ] COMPATIBILITY.md + docs/runtime.md
- [ ] Hardware check on Radxa + NanoPi: MountEMMC mounts eMMC not SD, persists
      across reboot; ErrNoEMMC on a board without eMMC; confirm sysfs
      `device/type` MMC/SD discriminator holds on both boards

## Quality gates
`go test ./...`, `go vet ./...`, `gofmt -l .` (empty), both
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`; cross-compile
the example for arm64 and `GOARCH=arm GOARM=6`.
