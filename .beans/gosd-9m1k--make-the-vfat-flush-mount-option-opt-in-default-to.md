---
# gosd-9m1k
title: Make the vfat flush mount option opt-in — default to normal writeback for write speed
status: completed
type: feature
priority: high
created_at: 2026-08-02T13:59:07Z
updated_at: 2026-08-02T13:59:07Z
---

JP (2026-08-02): forcing `flush` on every vfat mount tanks write speed and
is too intense a requirement for most applications; normal Linux
writeback (~30s dirty_expire) is good enough. Decisions locked in
conversation:

- **Default: no flush.** Apps needing durability already must use the
  documented fsync-based durable-write sequence (docs/runtime.md "Making
  a write durable"), which works identically with or without flush —
  bean gosd-0nk4 proved flush never covered renames anyway. Sloppy
  writers get standard Linux semantics.
- **Scope: all vfat DATA mounts** — /data (boot/mounts.go:207) and the
  emmc/disk shared path (internal/blockmount/platform_linux.go's
  vfatMountOptions). The GOSD-BOOT mount is out of scope (negligible
  write traffic, provisioning writes already bracketed carefully) — keep
  its behavior unchanged and note why. exFAT never took flush; untouched.
- **Config surface: build flag + gosd.toml override.** A `gosd build`
  flag (suggest `--data-flush`, bool, default false) baked into
  config.json via initcfg; a gosd.toml key (suggest top-level
  `data_flush = true`, absent = baked default) as the card-editable
  override, following the hostname/[env] pattern. Malformed values: log
  and use the baked default — never stop boot.

Architectural note: emmc/disk mount from the APP's process, not
gosd-init's, so they can't read config.json/gosd.toml directly. Use the
documented init→app channel: gosd-init computes the effective setting
(baked default + gosd.toml override) and exports a reserved env var
(suggest GOSD_DATA_FLUSH=1/0) alongside GOSD_BOARD/GOSD_HOSTNAME;
blockmount's mount path reads it. Verify the boot ordering: gosd.toml is
parsed after GOSD-BOOT mounts — confirm /data's mount happens after that
parse so the override applies to it (it should; verify in sequence.go),
and keep the decision logic pure/fake-testable (helper mapping
flush→mount options) per the platform-seam convention.

Docs in the same PR (behavior and docs change together): runtime.md's
storage section ("mounted with the flush option so data reaches the card
promptly" — now conditional), the durable-write section's flush
reference, the env-var table (new GOSD_DATA_FLUSH row), and the reserved
gosd.toml key. Grep COMPATIBILITY.md footnotes for flush mentions.

Tests: pure option-mapping helper; boot-sequence test that gosd.toml
override reaches the /data mount and the app env; blockmount honors the
env var; malformed toml value falls back baked+logged. The qemu CI jobs
stay green (hello's counter uses the durable-write pattern, unaffected
by flush).

## Todos

- [x] --data-flush flag → config.json (initcfg), default false
- [x] gosd.toml data_flush override, effective-setting computation in boot
- [x] GOSD_DATA_FLUSH env to app; blockmount reads it for emmc/disk mounts
- [x] /data mount honors effective setting; boot ordering verified
- [x] Docs: runtime.md storage/env/durable-write + gosd.toml reference

## Summary of Changes

- **Flag → config.json**: `gosd build --data-flush` / `gosd run --data-flush`
  (bool, default `false`) bakes `initcfg.Config.DataFlush` via
  `pipeline.Options.DataFlush`. Deliberately excluded from
  `ComputeIdentity`'s payload like `DataExpand` (config.json is excluded
  wholesale — see that docstring, now naming both fields); pinned by
  `TestBuildIdentityUnaffectedByDataFlush`
  (cmd/gosd/build_integration_test.go) and
  `TestAssembleBakesDataFlushIntoConfigJSON` (internal/pipeline).
- **gosd.toml override**: top-level `data_flush` key,
  `gosdtoml.Config.DataFlush *bool` (nil = absent = use the baked default).
  Parsed leniently, mirroring `[env]`'s coercion the other way around: a
  bare boolean is used as-is, a quoted `"true"`/`"false"` is coerced with a
  warning, anything else falls back to the baked default with a warning —
  never fails the rest of the file's parse.
- **Boot-ordering finding**: verified, not changed. `sequence.go`'s `Run`
  already parses `gosd.toml` (step 5, right after `MountBootPartition`)
  before `mountData` runs — `effectiveDataFlush` is computed right there,
  so the override reaches the `/data` mount without any reordering. It's
  computed once and threaded through explicitly (not re-derived from
  `gosdToml` later) because `ProvisionSnapshot`'s restore can reset
  `gosdToml.DataFlush` to nil afterwards — `DataFlush` isn't part of the
  provisioning snapshot (only Hostname/Wifi/Env are), matching
  docs/runtime.md's existing "what does not come back" wording.
- **Env channel**: `GOSD_DATA_FLUSH=1`/`0`, gosd-init's third fixed env var,
  already covered by `mergeUserEnv`'s existing `GOSD_*` reserved-prefix
  filter (confirmed, not duplicated). `internal/blockmount`'s new
  `vfatMountOption(getenv)` (mountoptions.go, no build tag — pure,
  macOS-testable) reads it back for `emmc`/`disk` FAT32 mounts, since they
  mount from the app's own process and can't read config.json/gosd.toml
  directly; `platform_linux.go`'s `mountData` is the one-line syscall-side
  caller. `cmd/gosd-init/internal/boot/mounts.go` gets the mirror-image
  pure helper, `dataMountOption(flush bool)`.
- **GOSD-BOOT untouched**: it never used `flush` (mounted read-only); a
  doc comment on `MountBootPartition` now says why it's out of scope
  (read-only makes close(2) writeback moot; its own writes are already
  synced where it matters).
- **Docs (docs/runtime.md)**: env var table gains `GOSD_DATA_FLUSH`;
  "Persistent storage: `/data`" explains the opt-in flag/key and the
  speed-vs-writeback trade, linking "Making a write durable"; that
  section's flush reference is now conditional (flush was never sufficient
  for rename durability even when it *was* unconditional — gosd-0nk4); the
  `emmc`/`disk` sections note they honor the same setting (exFAT
  unaffected). `COMPATIBILITY.md` has no flush footnotes to update
  (checked).
