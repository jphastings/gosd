---
# gosd-tdcc
title: Onboard eMMC format-and-mount (`emmc` package)
status: in-progress
type: task
priority: normal
created_at: 2026-07-10T06:16:35Z
updated_at: 2026-07-11T06:24:54Z
parent: gosd-jge2
blocked_by:
    - gosd-899s
    - gosd-0s0m
---

App-facing helper that lets a GoSD app use the onboard eMMC on the Rockchip
boards (Radxa Zero 3E, NanoPi Zero2): format it on first use, mount it on every
boot after. The soldered eMMC ships blank and can't be formatted on another
machine, so this formats in place — viability proved by [[gosd-0s0m]], decision
to invest recorded in [[gosd-899s]].

## Locked design (2026-07-10, agreed with JP)
- New public package `github.com/jphastings/gosd/emmc` (semver-relevant).
- Single entry point returning an error channel (fire-and-forget, one value):

      func FormatAndMount(label, mountpoint string, destructive bool) <-chan error

- **label** doubles as the idempotency key: an eMMC already FAT-formatted with
  this label is only mounted, never reformatted, so re-runs never wipe their
  own data. FAT volume label, ≤11 ASCII chars, matched case-insensitively.
- **Whole-device FAT** (mount source is `/dev/mmcblkN` itself, no partition
  table) — the only fs these kernels mount, and it avoids the privileged
  `BLKRRPART` reread.
- **destructive** guards existing *other* data. Blank eMMC (no fs + all-zero
  leading ~1MiB) is always formatted, even when false. A different-label FAT or
  non-FAT content: false → error and don't touch; true → wipe + reformat.
- Auto-discovers the eMMC via sysfs `device/type == MMC` (vs `SD`), and refuses
  the boot device (any mmcblk with a mounted partition) so booting from eMMC
  yields `ErrNoEMMC` rather than a wiped system. `ErrNoEMMC` is
  `errors.Is`-checkable; the channel delivers it where no eMMC is usable.
- App-imported library only; gosd-init stays unaware of eMMC (no `GOSD_EMMC`
  env var, no `gosd.toml` auto-mount) — a possible future follow-up.

## Structure (as built)
- `internal/emmcfmt` (from the spike, extended): `FormatFAT32` (whole-device
  FAT32 via go-diskfs) + `Inspect` (IsFAT/Label/Blank). Pure go-diskfs, no
  build tags, cross-platform tests on macOS.
- `emmc/emmc.go` — public API + pure `run` orchestration + `chooseEMMC`
  discovery decision + label validation; no build tag, fully fake-tested.
- `emmc/platform_linux.go` — real sysfs enumeration, `/proc/mounts` parse,
  `unix.Mount` (vfat, nosuid/nodev, `flush`).
- `emmc/platform_other.go` — stubs so it builds/tests on macOS.
- `examples/emmcstorage` — worked example, graceful `ErrNoEMMC` degrade,
  temp-file+fsync+rename durable write; cross-compiles arm64 + armv6 (dir named
  `emmcstorage` to avoid colliding with the top-level `emmc/` package on
  `go build ./examples/...`).
- `COMPATIBILITY.md` eMMC row + footnotes; `docs/runtime.md` eMMC section; CI
  example cross-compile lines.

## Todo
- [x] `internal/emmcfmt.Inspect` (IsFAT/Label/Blank) + tests
- [x] `emmc` package: public `FormatAndMount`, pure `run`, `chooseEMMC`, label
      validation + sentinel `ErrNoEMMC`
- [x] platform_linux (sysfs/procmounts/unix.Mount) + platform_other stubs
- [x] Fake-driven behavioral tests (mount-only / format-blank / refuse-foreign
      / reformat-destructive / already-mounted / no-eMMC / bad-label /
      discovery incl. boot-device exclusion); pass on macOS
- [x] `examples/emmcstorage` (cross-compiles arm64 + armv6, degrades gracefully)
- [x] COMPATIBILITY.md + docs/runtime.md + CI build lines
- [ ] Hardware check on Radxa + NanoPi: FormatAndMount formats a blank eMMC and
      mounts it (not the SD), a re-run mounts without reformatting, a
      different-label/foreign eMMC errors under false and reformats under true,
      and `ErrNoEMMC` on a board without eMMC. Confirms the `BLKGETSIZE64`
      size path and sysfs `device/type` discriminator hold on real hardware —
      the one thing the fake/backing-file tests can't cover (mirrors the
      hardware-test tail on [[gosd-xelb]]).

## Quality gates
`go test ./...`, `go vet ./...`, `gofmt -l .` (empty), both
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`; example
cross-compiles for arm64 and `GOARCH=arm GOARM=6`.

## Summary of Changes
Code half complete; stays in-progress pending the on-hardware check (no boards
yet). Built a new public `emmc` package whose `FormatAndMount(label, mountpoint,
destructive)` discovers the onboard eMMC, formats it (whole-device FAT, pure Go
via go-diskfs) only when blank or when the caller opts into overwriting other
data, and mounts it read-write — idempotent across runs via the FAT label so an
app never wipes its own storage. Discovery excludes the boot device so it can't
nuke the running system. Backed by `internal/emmcfmt` (format + inspect) from
the [[gosd-0s0m]] spike, with fake-driven orchestration tests, an
`examples/emmcstorage` worked example, and COMPATIBILITY/runtime docs.
