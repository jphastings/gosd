---
# gosd-hkp7
title: 'gosd-kernel.toml: developer kernel config (fragment, patches, firmware)'
status: completed
type: feature
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T13:21:27Z
parent: gosd-47rm
blocked_by:
    - gosd-abya
---

Part of [[gosd-47rm]]. The declarative project-repo config for
`gosd build-kernel` ([[gosd-abya]]): what a developer checks in to say "my
kernel additionally needs X".

## Locked decisions

- File name `gosd-kernel.toml` (fits the `gosd.toml` naming surface). New
  `internal/kernelconfig` package mirroring `internal/gosdtoml`
  (BurntSushi TOML; strict on unknown keys — unlike `gosd.toml`, this is a
  developer-authored build input, typos must fail loudly with the offending
  key named).
- **Schema (v1):**

  ```toml
  [kernel]
  # optional; both must equal the running gosd's pins for now (see below)
  based-on = "v0.4.0"        # artifacts version this extends
  builder  = "docker"        # docker | podman; omit = auto-detect

  [kernel.pi-zero-2w]        # per-board section, keyed by board ID
  fragment = "kernel/pi-zero-2w.fragment"   # path, merged AFTER GoSD's
  patches  = ["kernel/patches/*.patch"]     # applied AFTER GoSD's

  [[firmware]]               # runtime firmware blobs the new driver needs
  url    = "https://example.com/blob.fw"
  sha256 = "…"
  dest   = "vendor/blob.fw"  # under /lib/firmware in the initramfs
  ```

- **Not in v1:** `[[module]]` (out-of-tree `.ko` builds) — that follows the
  loadable-modules decision bean; the schema deliberately leaves the table
  name reserved. `based-on` ≠ the CLI's pinned `internal/artifacts.Version`
  is an error in v1 (cross-version builds are a later problem); its value is
  recorded into `source.json`.
- Fragment semantics documented in the schema docs: merged after GoSD's
  fragment, so a developer line wins; the required-`=y` assertions from the
  KernelSpec still apply (a developer can add, and may extend assertions via
  their fragment content, but cannot silently drop GoSD's requirements —
  builds that lose a required symbol fail, naming it).
- `[[firmware]]` entries reuse the URL+sha256 fetch machinery
  (`internal/fetch`) and land in the image's firmware set like board WiFi
  blobs do; never re-hosted, per the third-party-blob decision.
- Validation is behavioral-tested: happy path, unknown key, bad sha256
  length, missing fragment file, unknown board ID (with the known IDs listed
  in the error).

## Todos

- [x] Schema + strict parser + validation errors
- [x] Wire into `build-kernel --config` (overlay + firmware set)
- [x] Firmware entries flow into the image build's firmware placement
- [x] Tests incl. rejection cases
- [x] Quality gates green



## Implementation notes (discovered during gosd-hkp7)

- **based-on is validated but NOT recorded into source.json.** The locked
  decision said based-on's value "is recorded into source.json"; on
  inspection, internal/kernelbuild.writeSourceJSON writes source.json as a
  bare `map[string]artifacts.ComponentSource` (keys like "kernel",
  "uboot"), and that exact shape is copied verbatim into manifest.json's
  `boards.<board>.source` by CI's package.sh, then parsed back by
  internal/artifacts.BoardFiles.Source — a schema genuinely consumed
  downstream, not just an informational file. Adding a sibling "based_on":
  "<string>" key would not decode as an artifacts.ComponentSource and would
  break that consumer. This isn't a clean seam, so based-on is validated at
  parse time (must equal internal/artifacts.Version) but is not threaded into
  kernelbuild.Options/source.json. If we want it recorded later, source.json
  needs its own wrapper type first (a small follow-up, not done here per the
  bean's own guidance to record rather than force a bad seam).



## Summary of Changes

- Grew `internal/kernelconfig` from the gosd-abya-seeded minimal parser to
  the full v1 schema: `[kernel].based-on`/`.builder`, per-board
  `[kernel.<board-id>]` fragment/patches (unchanged), and `[[firmware]]`.
  Parsing is strict throughout — an unrecognized key anywhere (top level,
  inside a board section, inside a firmware entry) is an error naming it,
  via a single `toml.MetaData.Undecoded()` pass after a manual
  `map[string]toml.Primitive` decode of `[kernel]` (needed since that
  table mixes fixed scalar keys with dynamically-named board subtables).
  `[[module]]` is rejected outright with the reserved-decision message.
  `based-on`, when set, must equal `internal/artifacts.Version`;
  `builder`, when set, must be `docker`/`podman`; firmware entries
  validate url/sha256(64 hex)/dest (relative, non-".."-escaping).
- `cmd/gosd build-kernel --builder` now falls back to
  `[kernel].builder` when the flag itself is unset (flag still wins) via
  the new `effectiveBuilderPref` helper.
- `gosd build` gained a `--kernel-config` flag (same
  gosd-kernel.toml-in-cwd default-discovery rule as build-kernel's
  `--config`). Its `[[firmware]]` entries are fetched/verified through
  `internal/fetch` (new `cmd/gosd/kernelfirmware.go`), cached under
  `<user-cache>/gosd/kernel-firmware` keyed by sha256, and merged into
  `pipeline.Options.ExtraFirmware` — a new map alongside each board's own
  `FirmwareFiles()` — so `internal/pipeline` stays free of any
  developer-config/network concerns; entries land at
  `/lib/firmware/<dest>` in the initramfs next to the board's own
  firmware. The common case (no gosd-kernel.toml) makes zero network calls,
  verified by a dedicated tripwire test alongside the existing
  `--artifacts-dir` ones.
- Tests: full-schema happy path; one rejection test per locked validation
  case (unknown key at each nesting level, unknown board ID listing known
  IDs, reserved `[[module]]`, based-on mismatch, invalid builder, bad
  sha256, absolute/".." dest, missing url/dest); `--builder` flag-vs-config
  precedence; a build-integration fixture test with an httptest.Server
  proving a firmware entry really lands under `/lib/firmware` in the built
  image's initramfs alongside the board's own firmware.
- See "Implementation notes" above: based-on is validated but not threaded
  into source.json (schema not cleanly extensible without breaking a real
  downstream consumer).
- All quality gates green: `go test ./...`, `go vet ./...`, `gofmt -l .`,
  `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`.
