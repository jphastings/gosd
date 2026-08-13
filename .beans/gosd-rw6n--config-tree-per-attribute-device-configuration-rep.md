---
# gosd-rw6n
title: 'Config tree: per-attribute device configuration replaces gosd.toml'
status: completed
type: epic
priority: normal
created_at: 2026-08-13T15:38:50Z
updated_at: 2026-08-13T20:29:20Z
---

Per-attribute device configuration as a directory tree on the boot partition,
replacing gosd.toml wholesale. Decided with JP 2026-08-12/13; supersedes the
unshipped `--env-placeholder` mechanism (PRs #268/#269, on main but never
released) and the closed whole-file approach (#271). **No migration path**:
nothing shipped, clean cut — v0.5.0 was never tagged and npm 0.2.0 never
published, and both stay held until this epic closes.

## Why

Every problem across three design rounds was a syntax problem: injection,
merging and documentation all operated on fragments of a parsed TOML document
(the `[env]` header-capture hazard, client-side TOML escaping, duplicate-key
parse errors, restate-everything-or-lose-it, comments destroyed by re-render).
One value per file has no syntax: injection is "write bytes into a published
region", per-attribute merging is "compare hashes", documentation is a sibling
file. The `.new` upgrade-conflict handling follows Debian's conffile
semantics — a proven design for exactly this problem.

## The format (LOCKED)

- A `config/` tree at the FAT root of the boot partition. One value per file;
  values are read newline-trimmed; empty after trim = unset ("enabled" means
  non-empty).
- Documentation sidecars: `<name>.explain.md` (markdown, per value) and
  `explain.md` (per directory/group). Required at `gosd build` time for every
  value file, env vars included — the app's own or inherited from gosd's
  defaults. NOT required at runtime: the pressure is on the developer, never
  the customer.
- Value names may contain periods (`whatever/google-service-account.json`).
  Reserved — refused at build, ignored at runtime: the `.new` and `.unused`
  suffixes, `explain.md` / `*.explain.md` as value names, leading-dot names,
  `._*` (macOS AppleDouble — these can otherwise defeat any sidecar gate,
  since macOS writes a `._X` for every `X` it touches on FAT), `Thumbs.db`,
  `desktop.ini`.
- FAT is case-insensitive: names colliding case-insensitively are refused at
  build (matters most for env names).
- **Padding is the reservation**: every value file is written padded with
  trailing newlines to at least 256 bytes; a file shipped larger reserves its
  own size (a value that must accept a 4KiB injection ships as 4096 newlines —
  reads as unset, reserves 4KiB). `explain.md` files are not padded and not
  injectable.
- **Feature pruning**: a feature's config directory is written only when the
  feature is present in the image — board capability AND build flags. No
  `config/ingress/cloudflared/` on pi-zero-w (armv6; cloudflared is
  arm64-only), nor on any build that didn't pass `--ingress cloudflared`.
- gosd ships its defaults as a real `config/` directory in this repo; the app
  supplies an overlay directory; per-file overlay, the app's file wins, and
  explanations are inherited when not overridden (an app can override
  `ingress/cloudflared/port` and keep gosd's `port.explain.md`).
- Paths keep the existing setting names: `hostname`, `wifi/ssid`,
  `wifi/passphrase`, `data_flush`, `env/<NAME>`,
  `ingress/cloudflared/{token,hostname,port}`,
  `ingress/tailscale-funnel/{authkey,hostname,port,funnel_port}`.
- config.json carries a per-value-file sha256 map of the tree as built.

## Runtime semantics (LOCKED)

- The tree is the single source of truth; config.json's baked values are the
  per-field fallback for unset values. The gosd.toml file, its parser and
  template, and the gosd.toml > cloud-init > config.json precedence chain are
  all deleted.
- Cloud-init consumption: read the seed, DELETE it (and sync) **before**
  writing its values into the tree — a crash in the gap loses wizard input
  (re-run the wizard), rather than a stale seed silently clobbering later
  hand-edits. Wizard values thereby become ordinary tree edits, which is what
  carries them across reflashes.
- `GOSD_*` env names: refused at build, ignored-and-logged at runtime.

## Persistence across reflash (LOCKED)

- A store on /data mirrors the tree. **Presence in the store IS the record of
  customer intent** — no old-default hashes are kept (JP's decision): `.new`
  therefore means "the current default you're overriding", refreshed on each
  reflash, not "the default changed since your edit". Empty defaults produce
  no `.new`, so the common secret-shaped values stay quiet.
- Each store entry commits as write value -> sync -> write digest -> sync; an
  entry whose value doesn't match its digest is torn and dropped (self-heal).
- The store records the image identity it was last written under. **The
  restore phase runs only when the running image's identity differs** — this
  is what makes "fresh flash -> restore" and "continuing install -> a revert
  counts" decidable, since the two are byte-identical per file.
- Restore (per file, only on identity change): card ≠ baked -> the card wins
  (injection or pre-boot hand-edit is the freshest intent). Card == baked and
  the store has an entry -> write the stored value to the card, and write the
  baked value alongside as `<name>.new` when it is non-empty and differs from
  the stored value (an existing `.new` is overwritten). Otherwise the default
  stands.
- Persist (every boot, after cloud-init consumption): card ≠ baked -> upsert
  into the store. Card == baked and a store entry exists -> DELETE the entry:
  **a hand-edit back to the default is treated as the default** (JP's
  decision — it is indistinguishable from one, and no `.new` marks it).
- Orphans: a stored entry (outside `env/`) whose path is absent from the new
  baked tree is written to the card as `<name>.unused` and removed from the
  store — one reflash window to retrieve the value, documented as such.
  `config/env/` is exempt: customer-created env vars are never in the baked
  tree, so absence there is their normal state and they restore normally.
- Explanations are image-owned: never persisted, never restored, no `.new`.
- No data partition -> no store -> nothing survives a reflash (unchanged
  expectation; build with `--data-size=expand`).

## Injection (LOCKED)

- The manifest gains a top-level `config` array: `{path, size, sha256,
  ranges, value}` per value file — the placeholder shape plus `value`, the
  trimmed pristine value, so tooling can show defaults without FAT parsing
  (the same bytes are public in the .img). `--placeholder` is unchanged and
  remains the mechanism for app-owned files that are not configuration.
- TypeScript: `withPlaceholders` gains `config: Record<string, string>` —
  keys are tree paths without the `config/` prefix (`"wifi/ssid"`,
  `"env/API_TOKEN"`), values padded with trailing newlines to the region
  size. Refused before anything downloads: an unknown path (error lists what
  the image has), a value longer than its reservation, an `env/GOSD_*` key,
  an image with no config entries. The `env` option, `renderEnvBody` and all
  TOML machinery are deleted — no TOML anywhere in the client contract.

## Build flags (LOCKED)

`--env` and `--env-file` are removed (they collide with the explain.md build
gate; the overlay dir is the input). `--hostname`, `--wifi-ssid` and
`--wifi-password` are removed too — the overlay dir serves the developer and
the Imager wizard serves the customer. The app overlay is
`gosd build --config-dir <dir>`, defaulting to a `config/` directory adjacent
to the app's main package when one exists.

## Release

Nothing tags until this epic closes: v0.5.0 stays untagged and npm stays
unpublished. The dead `npm/gosd/v0.2.0` tag stays dead; the client ships as
0.3.0 in its own bump PR afterwards.

## Children

- Build side + manifest + TypeScript client (ONE PR: CI regenerates the
  fixture from the Go code and runs the TS integration test against it, so
  the two halves cannot land apart — proven twice)
- gosd-init read side (blocked by the build side)
- The config store (blocked by the read side)
- Docs (blocked by the store; written as if the tree always existed, per JP —
  no mention of the previous format anywhere)

## Summary of Changes

Shipped across four PRs, one per child bean, implemented 2026-08-13:

- **#273** (gosd-cn4p): `internal/configtree` — embedded defaults tree,
  `--config-dir` overlay, build-gate validation, padding-is-reservation,
  per-board/per-flag feature pruning, `configDigests` in config.json, the
  manifest `config` array, `--env-placeholder` and
  `--env`/`--env-file`/`--hostname`/`--wifi-ssid`/`--wifi-pass` removed;
  TypeScript client's `config` option replaces `env`/TOML wholesale.
- **#274** (gosd-ypkv): `cmd/gosd-init/internal/cardconfig` reads the tree;
  cloud-init seed consumed (read → durable delete → write into tree);
  `internal/gosdtoml`, provsnapshot and the gosd.toml boot file deleted.
- **#275** (gosd-87ip): `cmd/gosd-init/internal/configstore` on /data —
  identity-gated restore, `.new`/`.unused`, revert-is-default,
  write → sync → digest → sync commits; qemu reflash CI extended end-to-end.
- **#277** (gosd-fdt2): docs rewritten as if the tree always existed
  (docs/config.md replaces docs/gosd.toml.md); CLAUDE.md decisions updated.

Notable in-flight decisions: defaults ship as EMPTY value files (unset →
config.json's baked per-field fallback; hostname stays the sanitized app
name); the store keeps parallel values//digests/ trees because value names
may contain periods; `wifi/hidden` was dropped (not in the locked path
list); a restored `data_flush` takes effect one boot later.

## Correction (2026-08-13, after close)

The "nothing shipped" premise above was wrong when written: CLI v0.5.0 (with
the old single-file config and the `--env-placeholder` mechanism) was tagged
and released 2026-08-12, and npm 0.2.0 was published to the `next` dist-tag
(never `latest`). Numbering therefore moves on: the config tree ships as CLI
v0.6.0 and npm 0.3.0 (bean gosd-pnl2). Whether the v0.6.0 release notes call
out the break for v0.5.0 users is JP's call at tag time.
