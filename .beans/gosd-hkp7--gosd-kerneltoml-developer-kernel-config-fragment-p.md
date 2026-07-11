---
# gosd-hkp7
title: 'gosd-kernel.toml: developer kernel config (fragment, patches, firmware)'
status: todo
type: feature
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
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

- [ ] Schema + strict parser + validation errors
- [ ] Wire into `build-kernel --config` (overlay + firmware set)
- [ ] Firmware entries flow into the image build's firmware placement
- [ ] Tests incl. rejection cases
- [ ] Quality gates green
