---
# gosd-ypkv
title: gosd-init reads the config tree; gosd.toml deleted
status: completed
type: feature
priority: normal
created_at: 2026-08-13T15:39:37Z
updated_at: 2026-08-13T17:17:52Z
parent: gosd-rw6n
blocked_by:
    - gosd-cn4p
---

The read half of epic gosd-rw6n (which holds all locked decisions): gosd-init
consumes the `config/` tree and gosd.toml is deleted everywhere.

Note for reviewers: this bean also deletes provsnapshot (it re-renders
gosd.toml, which no longer exists). Between this bean and the store bean,
main has NO reflash persistence — acceptable because every release is held
until the epic closes.

## Todos

- [x] Tree enumeration with the reserved-name/junk filtering from the epic;
      values newline-trimmed; empty = unset, falling back per field to
      config.json's baked values
- [x] `env/` -> app environment (GOSD_* ignored and logged; redaction rules
      built from the merged values exactly as today); `wifi/ssid` +
      `wifi/passphrase` -> wifiup; `hostname` (config.json's sanitized
      default when unset — preserving gosd-4hz1's wizard-can-win behaviour
      naturally); `data_flush`; `ingress/*` (present only when the feature
      shipped in this image)
- [x] Cloud-init consumption: read seed -> delete + sync -> write values into
      the tree; a crash loses wizard input, never clobbers later edits
- [x] Delete gosd.toml: the template and parser in internal/gosdtoml (the
      cloud-init YAML parsing in internal/provision stays), pipeline's
      gosd.toml boot file, provsnapshot, and every reference in gosd-init
- [x] Crash-report source summaries (describeEnvSources) reworded for the
      tree
- [x] Fake-driven tests that pass on macOS, per the repo's platform seam
      shape; the qemu boot-to-HTTP CI job must stay green
- [x] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
      `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`

## Summary of Changes

gosd-init now reads the card's `config/` tree as the single source of truth,
and gosd.toml is gone from the repo.

- **`cmd/gosd-init/internal/cardconfig`** is the runtime half of the format:
  `Read(dir, log)` walks `/boot/config`, skipping documentation, the
  device's own `.new`/`.unused` files and operating-system junk via a new
  `configtree.IgnoredName` (the runtime twin of the build's `checkName` —
  a test pins the two against each other), and returns a `Tree` of
  `configtree.Value`s, so each setting carries both the bytes the card holds
  (what the store will hash) and the newline-trimmed value it reads as.
  `Tree.Write` writes settings back, padded to the reservation the file
  already holds, through `durable.WriteFile`; it updates the in-memory tree
  whether or not the card accepted the write. A file larger than
  `MaxValueBytes` (64KiB) is skipped rather than read into a device whose
  whole rootfs is RAM.
- **Settings flow** (`boot/sequence.go`): `hostname` (validated as before,
  refused rather than mangled, falling back to config.json's baked default),
  `wifi/ssid` + `wifi/passphrase` -> wifiup, `data_flush` (non-empty means
  on — the card can't turn a baked flush off), `env/<NAME>` merged per name
  over config.json's baked env with `GOSD_*` ignored-and-logged, and
  `ingress/*` read by each agent. `describeEnvSources` now reads
  "app env: API_URL (config/env); PORT (baked)".
- **Cloud-init is consumed, not consulted**: `provision.Result.SeedFiles` +
  `provision.DeleteSeed`, and `boot.Deps.EditBoot` (a boot-partition
  read-write window, `Platform.EditBootPartition`) run twice — the seed is
  deleted and that deletion made durable in the FIRST window, the values are
  written in the SECOND. A crash between them loses the wizard's answers
  (re-flash to give them again); nothing can leave a seed that overwrites
  later hand-edits. Sequence tests assert the card's state at the end of each
  window. The wizard's answers overwrite settings already in the tree — the
  most recent statement of intent wins, which is what keeps gosd-4hz1's fix
  alive when an app ships a non-empty default.
- **Deleted**: `internal/gosdtoml` (template + parser), pipeline's gosd.toml
  boot file, `cmd/gosd-init/internal/provsnapshot`, `Platform.WriteBootFile`,
  and every gosd.toml reference in Go code, comments and the injectfixture
  fixture. `WriteFileDurably` moved to `cmd/gosd-init/internal/durable` (the
  boot counter and the tree writes share it); it now writes through a
  DOT-prefixed temp name, so a power cut mid-write can't leave a
  `hostname.tmp` that the tree reader would take for a setting.
- **The boot-partition sentinel** (`MountBootPartition`) moved from
  `gosd.toml` to `config/explain.md`: the one file of the tree no feature
  pruning can remove, written for every board.
- **Ingress**: `cloudflared.Config` / `tsfunnel.Config` replace the gosdtoml
  structs and hold text, with each package's `resolveMode` parsing and
  refusing ports itself; every log line names the file to open
  (`config/ingress/cloudflared/port ...`). CI's two baked-but-not-configured
  assertions were updated to match.

Deviations worth review:

- **`wifiup.Credentials.Hidden` is gone.** Only cloud-init's network-config
  could express a hidden SSID, and the tree (whose paths are locked) has no
  `wifi/hidden`. Keeping a flag that could only ever apply on the first boot
  after a flash would have been worse than dropping it; bean gosd-lbpm, if
  taken up, should add a real setting for it.
- **`--data-flush` can no longer be turned OFF from the card** (empty means
  unset, so a baked `true` stands). That follows directly from the locked
  "empty = unset" semantics; the old `data_flush = false` override has no
  expression in the tree.
- Docs still describe gosd.toml: they belong to the docs bean (gosd-fdt2),
  which the epic sequences after the store.
