---
# gosd-x3j5
title: 'Unify gosd build/run image-content wiring: run.go silently misses new pipeline content'
status: completed
type: task
created_at: 2026-08-07T13:58:38Z
updated_at: 2026-08-07T17:00:00Z
---

Found during gosd-kzgq (2026-08-07): `gosd run` (cmd/gosd/run.go) constructs
its OWN `pipeline.Options` — it does not share `runBuild`'s construction — so
any new image content wired into `gosd build` (ExtraFiles, ExtraFirmware
additions, future ingress binaries...) must be duplicated by hand in run.go
or qemu/CI images silently diverge from flashed images. The CA bundle was
nearly shipped build-only; the review pass caught it. This class of bug is
invisible until someone diffs a qemu image against a real one.

## Fix

Extract the shared, board-independent parts of `pipeline.Options`
construction into one helper both `runBuild` and `runRun` call, so new
content lands in both by construction. First audit which fields differ
INTENTIONALLY today (run's fixed board, console wiring, sizing defaults) and
keep those explicit at the call sites, not hidden in the helper.

Behavioral test: build an image via each path with the same inputs and
assert the initramfs extra-content set (everything except board-specific
boot files) is identical — the test is the durable guard; the helper is just
how it passes.

## Todos

[x] Audit which `pipeline.Options` fields differ INTENTIONALLY between
    `runBuild` and `runRun` today, vs which are duplicated identical
    construction that belongs in a shared helper
[x] Extract the shared, board-independent content resolution (the CA bundle)
    into one helper both `runBuild` and `runRun` call
[x] Behavioral test: `gosd build --board=qemu-virt` and `gosd run` produce
    identical initramfs content from matching inputs, except the one field
    expected to vary (config.json's build timestamp)
[x] Confirm the test actually catches the regression class described above
    (verified locally by reverting the run.go wiring and watching it fail)

## Summary of Changes

- **Audit** (bean's first Fix step): fields that differ INTENTIONALLY today
  and stay explicit at each call site, not folded into a shared helper —
  `Board` (run is hardcoded to qemu-virt; build iterates every selected
  board), `Config`'s WiFi/UsbGadget/Env/ConsoleBaud (`gosd run` has no flags
  for these — always zero values), `DataSizeBytes`/`DataExpand` (run always
  builds with `defaultDataSize="0"`, no `--data-size` flag), `ExtraFirmware`
  (build-only, from `--kernel-config`), `ExtraExecutables` (build-only, from
  `--with-external`), `Placeholders` (build-only), and every per-invocation
  path (`AppBinaryPath`/`InitBinaryPath`/`OutputPath`). `BootSizeBytes` and
  `DataFlush` already had parallel, independently-defined flags with matching
  defaults on both commands — no shared construction to extract there, just
  two flags that happen to agree.
- The one piece of **duplicated, board-independent construction** was the CA
  bundle (bean gosd-kzgq): `caCertsCacheDir` + `resolveCACerts` +
  `openCACertsForBoard`, called identically in both `runBuild` and `runRun`
  to build `pipeline.Options.ExtraFiles`. Added
  `cmd/gosd/sharedcontent.go` (`sharedContent` type,
  `resolveSharedContent(ctx, artifactsDir)` resolving it once per invocation,
  `openSharedContent(shared)` opening a fresh reader set per board —
  mirroring the resolve-once/open-per-board split `kernelfirmware.go` and
  `buildexternal.go` already use) as the one place this construction lives
  now. Both `cmd/gosd/build.go`'s `runBuild` and `cmd/gosd/run.go`'s `runRun`
  call it instead of duplicating the CA-cert wiring, and no longer import
  `internal/cacerts` or `io` directly for this purpose — any content added to
  `sharedContent` in the future reaches both commands by construction.
- Added `cmd/gosd/buildrun_parity_integration_test.go`
  (`TestBuildAndRunProduceIdenticalInitramfsContent`): builds the same app
  via `gosd build --board=qemu-virt` and `gosd run` (the only board `gosd
  run` ever targets, so there's no board-specific-boot-file set to exclude —
  qemu-virt's initramfs already carries no board-specific content) with
  matching `--artifacts-dir`/`--hostname` flags, decodes both initramfs
  archives, and asserts an identical entry set with identical mode and
  content for every entry except `etc/gosd/config.json` (whose baked build
  timestamp is expected to differ between any two builds). Verified this
  test actually catches the bug class the bean describes: reverting it to
  drop `run.go`'s `ExtraFiles` wiring locally made the test fail with a
  clear diff (`etc/ssl/certs/ca-certificates.crt` present in build's
  initramfs, absent from run's); go build output across repeated
  invocations of the same source/flags was confirmed byte-identical, so the
  content-equality assertion isn't flaky on that account.
