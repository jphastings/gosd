---
# gosd-hcca
title: Read-only /data fallback; drop GOSD_DATA env var
status: in-progress
type: task
priority: normal
created_at: 2026-07-10T03:52:27Z
updated_at: 2026-07-10T04:13:30Z
---

Supersedes the `GOSD_DATA` env-var half of the data-partition contract
locked in gosd-xelb. Approved by JP 2026-07-10: gosd is not yet released
publicly, so the behaviour change carries no compatibility cost.

## Why

Today gosd-init mounts GOSD-DATA read-write at `/data` and signals
availability via a `GOSD_DATA` env var; the app is expected to gate writes
on it. Two problems:

1. The env var's only real job is a *presence* signal — the path is already
   the hardcoded constant `/data` (`cmd/gosd-init/main.go`), never dynamic.
2. If the partition is absent (`--data-size=0`) or the mount fails at
   runtime (bad card, device didn't appear before the timeout), `/data`
   stays an empty **writable** directory on the RAM-backed initramfs rootfs.
   An app that writes there — especially via the ubiquitous
   `os.MkdirAll(filepath.Dir(path))` idiom — gets phantom success: writes
   land in RAM and vanish on the next reboot, silently.

Permission bits can't fix (2): `/app` runs as root (`exec.Command` with no
credential change), and root's `CAP_DAC_OVERRIDE` bypasses DAC permission
checks, so a mode-000 `/data` stops nobody. A read-only *mount*, by
contrast, returns `EROFS` at the VFS layer before permission checks are
reached — capabilities don't override a read-only superblock.

## Locked decision (this bean)

- When the real GOSD-DATA partition mounts, `/data` is the writable FAT,
  exactly as today.
- When it is absent or unmountable, gosd-init mounts an empty **read-only
  tmpfs** over `/data` instead of leaving it writable. Any write then fails
  immediately with `EROFS` — a loud, honest error at the write site —
  instead of silently vanishing on reboot. Still never fatal to boot.
  (tmpfs is already available; it backs `/run` in the early mounts.)
- The `GOSD_DATA` env var is **removed entirely**. `/data` is the fixed,
  documented path; an app built with persistence writes there, and if this
  boot has no partition it gets `EROFS`. `GOSD_*` remains a reserved
  namespace (`GOSD_BOARD`, `GOSD_HOSTNAME` stay).
- Behaviour shift from gosd-xelb: "boot proceeds, app silently gets no
  persistence" becomes "boot proceeds, writes to `/data` fail loudly when no
  partition". This is the intended trade, and it catches the most valuable
  case — an app that correctly assumed persistence but hit a bad card.

## Todos

- [x] gosd-init: on data-mount failure/missing, mount an empty read-only
  tmpfs at `/data` (`MS_RDONLY|MS_NOSUID|MS_NODEV`) instead of returning ""
  and leaving it writable; keep it non-fatal
  (`boot/sequence.go` `mountDataPartition`, helper in `boot/mounts.go`)
- [x] gosd-init: stop appending `GOSD_DATA` to the app env
  (`boot/sequence.go`); update the `reservedEnvPrefix` and
  `Options.DataTarget` doc comments that reference it
- [x] Tests: `boot/sequence_test.go` + `boot/mounts_test.go` — assert no
  `GOSD_DATA` in the app env in any case, and assert the read-only fallback
  mount is performed (fake Mounter records a read-only tmpfs at `/data`)
  when the partition is missing/unmountable
- [x] examples/hello: write the boot counter to `/data` directly; on write
  error (`EROFS` when no partition) report "no-data-partition" gracefully
  rather than gating on the env var. Stays stdlib-only; cross-compiles for
  arm64 and `GOARCH=arm GOARM=6`
- [x] docs/runtime.md: rewrite the `GOSD_DATA` env-var entry and the
  "Persistent storage: /data" section for the new model (always `/data`;
  read-only + `EROFS` when no partition; no env var; drop the "gate on
  GOSD_DATA" guidance); remove `GOSD_DATA` from the reserved-names example
- [x] docs/design/ab-updates.md + COMPATIBILITY.md: confirm/adjust any
  `GOSD_DATA` wording (the COMPATIBILITY row is about the partition itself,
  likely unchanged — verify)
- [x] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
  `golangci-lint run ./...` AND `GOOS=linux golangci-lint run ./...`

## Acceptance

- With a data partition: examples/hello persists its counter across reboots
  (unchanged from gosd-xelb).
- With `--data-size=0`: a write to `/data` fails with `EROFS`; examples/hello
  reports "no-data-partition"; boot is otherwise normal.
- No `GOSD_DATA` appears in `/app`'s environment in either case.
- gosd-xelb's pending hardware torture-test todo is unaffected.

## Notes

- Optional, out of scope here: once a public `device/` package lands, an
  exported `/data`-path constant would spare apps a magic string. That
  package doesn't exist yet, so not part of this bean.



## Summary of Changes

- **cmd/gosd-init/internal/boot/mounts.go** — new `MountDataReadOnlyFallback`
  mounts an empty read-only tmpfs (`MS_RDONLY|MS_NOSUID|MS_NODEV`) over the
  data target. The doc comment records *why* it's a read-only mount and not
  restrictive permission bits: `/app` runs as root, whose `CAP_DAC_OVERRIDE`
  bypasses DAC checks, but `EROFS` from a read-only superblock is enforced at
  the VFS layer and no capability overrides it.
- **cmd/gosd-init/internal/boot/sequence.go** — `mountDataPartition` (returning
  the dir for the env var) became `mountData`, which mounts the writable FAT
  when the partition is present and otherwise mounts the read-only fallback so
  a stray write fails loudly instead of vanishing from the RAM rootfs. Still
  never fatal. Dropped the `GOSD_DATA` env-var export entirely; updated the
  `Options.DataTarget` and `reservedEnvPrefix` doc comments.
- **examples/hello** — writes the boot counter to the fixed `/data` path (no
  more `os.Getenv("GOSD_DATA")`); an `EROFS` write (image built `--data-size=0`)
  is reported as "no-data-partition" rather than a failure. Stdlib-only; still
  cross-compiles for arm64 and `GOARCH=arm GOARM=6`.
- **docs/runtime.md** — removed `GOSD_DATA` from the env-var and reserved-name
  lists; rewrote "Persistent storage: /data" for the new model (fixed `/data`
  path, read-only + `EROFS` when no partition, treat `EROFS` as "no persistence
  this boot").
- **Tests** — `boot` unit tests assert no `GOSD_DATA` is ever exported, and
  that a read-only tmpfs is mounted at `/data` when the partition is missing;
  added a focused test for `MountDataReadOnlyFallback`.

Supersedes the `GOSD_DATA` env-var half of gosd-xelb's locked decision (JP
approved 2026-07-10; gosd not yet released publicly). gosd-xelb's pending
hardware pull-power torture test is unaffected.
