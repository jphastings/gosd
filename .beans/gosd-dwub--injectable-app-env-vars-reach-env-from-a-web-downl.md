---
# gosd-dwub
title: 'Injectable app env vars: reach [env] from a web-downloaded image'
status: completed
type: feature
priority: normal
created_at: 2026-08-12T10:25:48Z
updated_at: 2026-08-12T13:09:49Z
---

Today a downloader can splice any `--placeholder` file into an image, but NOT the
one setting most apps actually want: `gosd.toml [env]`. App env has exactly two
sources (`docs/runtime.md`): baked defaults in `config.json` — inside the
compressed initramfs, so no stable byte ranges exist — and the card's
`gosd.toml [env]` table, which `internal/pipeline` refuses as a `--placeholder`
path (case-insensitive collision against existing boot files). Cloud-init carries
hostname/WiFi only, never env.

The workaround that exists today (app reads a placeholder file from `/boot` and
calls `os.Setenv` itself) is being documented under bean gosd-5pz2, but it costs
every app ~15 lines of loader code and silently forfeits gosd-init's automatic
crash-report redaction of env values (`envRedactionRules` only sees the merged
`[env]`/baked map). Both problems disappear if the injected values arrive through
gosd-init's normal env merge.

## Two designs — JP to choose before any code (NOT yet locked)

### Option A — pad `[env]` inside gosd.toml and report its sub-range

A build flag (e.g. `--env-placeholder <size>`) pads the rendered `[env]` section
body to exactly `<size>` bytes with `#` comment padding, and the manifest gains
the byte ranges of that body.

- Reuses the existing verbatim-body splice path (`gosd build --env-file` already
  writes a developer-authored `[env]` body through `gosdtoml.EnvSection.Verbatim`).
- Needs new machinery: `image.WriteReport.FileRanges` reports whole-file ranges, so
  the manifest writer must map a logical sub-span of gosd.toml onto its ordered
  cluster ranges, and `gosdtoml.Render` must return where the body landed.
- Needs a manifest schema addition — a distinct top-level key (e.g. `"env"`), NOT a
  pseudo-path in `placeholders[]`, whose `path` grammar is `[A-Za-z0-9._-]+` and
  whose entries the npm package keys by path.
- Pushes TOML rendering into every client: `KEY = "value"` with correct escaping,
  padded to the exact size. The npm package is deliberately zero-dependency, so
  that is hand-written there too.
- Nothing changes on-device; gosd-init reads `[env]` exactly as it does now.

### Option B (recommended) — gosd-init reads a designated env file from /boot

`gosd-init` reads a fixed boot-partition path (e.g. `/boot/gosd.env`) if present
and merges it into the app env; the developer reserves it with the ordinary
`--placeholder gosd.env=<size>` and the client injects it like any other
placeholder.

- No manifest change, no sub-range mapping, no new build flag: the existing
  documented `--placeholder` contract carries it unmodified.
- File format can be the SAME `[env]` body format `--env-file` already takes, so
  `gosdtoml.ParseEnvBody` parses it — no new parser, no new grammar, and a client
  that can write `KEY = "value"` works for both designs.
- Redaction and reserved-name handling come free: the values join `mergeUserEnv`'s
  output, which is what `envRedactionRules` and the `GOSD_*` refusal already act on.
- Apps need no code at all.
- Costs: a new source in the boot-time precedence chain (proposed
  `config.json` < `/boot/gosd.env` < `gosd.toml [env]`, per key, so a hand edit
  still wins over an injected value — the most local, most deliberate act wins,
  matching the existing chain's logic), a pristine-placeholder check in gosd-init
  (a file still starting `# GOSD-PLACEHOLDER` is comment-only and parses to an
  empty table, so this likely falls out for free — verify), and one more file the
  boot partition documents.

## Open questions

- Option B's filename and whether it is fixed or named by a `gosd.toml` key.
- Whether the injected values should be visible in the boot log's `app env:`
  source summary (`describeEnvSources`) as a third origin — almost certainly yes.
- Whether the npm package grows a helper that renders a `[env]` body from a JS
  object (both options need the same escaping; it is the one piece of new client
  code either way).

## Todos

- [x] JP picks Option A or B (or rejects both); record the decision here before coding
- [x] Implement the chosen design
- [x] Tests: end-to-end build -> patch ranges -> boot-sequence env merge (fake-driven, macOS-safe), pristine-placeholder-is-absent, `GOSD_*` refusal still applies to injected keys, redaction rules cover injected values
- [x] Docs: fold into the image-injection and gosd.toml/runtime env docs; update the workaround section gosd-5pz2 adds
- [x] npm package support: split out as bean gosd-ypyz — the JS client keys everything off `placeholders[].path` and needs a TOML-body renderer of its own, which is a self-contained piece of work in a separately-gated area
- [x] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`


## LOCKED: Option A, decided 2026-08-12

JP chose Option A. What settled it was field feedback from the Backup.ist/atfs
developer, who hit the gap the workaround leaves: an injected placeholder file
is invisible to `gosd-init`, so it is outside the provisioning snapshot, so a
reflash wipes it. Their app had to compensate with "injected fills only a key
the environment leaves empty", which in turn forced an example value
(`ATFS_OWNER_DID = "did:…"`) to become `""` — an example is indistinguishable
from a set value, so it would have shadowed every injected DID forever.

**Option A inherits both behaviours from code that already ships, with no new
classification rules.** `provsnapshot`'s restore is per key
(`provsnapshot.go`, the `snap.Effective.Env` loop): a key is restored only if
the snapshotted value differed from the baked default it was taken against
(snapshot intent) AND the new card's `gosd.toml` value does not differ from
this image's baked default (no fresh intent). Under Option A an injected value
IS a `gosd.toml` value, so:

- Freshly injected card: the region differs from `config.json`'s baked
  defaults -> fresh intent -> restore blocked -> the injection applies, and
  that boot's snapshot is refreshed with it.
- Plain reflash of a pristine image: the region renders byte-identical to the
  baked defaults -> no fresh intent -> the snapshot restores the operator's
  previous value. Over-flash recovery.
- Hand-edit beats both, unchanged.

Option B would need `provsnapshot`'s fresh-intent test extended to the injected
file; miss that and the restore writes the old value into `gosd.toml`, which
outranks the injected file, and every future injection is shadowed by the
first — the reporter's own bug, one layer down. That risk is what tipped the
choice.

### Locked design

- **Flag:** `gosd build --env-placeholder <size>` (e.g. `8KiB`), reserving
  exactly `<size>` bytes for the `[env]` section BODY. Composes with `--env`
  and `--env-file`, whose rendering becomes the region's pristine content.
- **The region is the WHOLE `[env]` body**, not a padded block appended below
  the rendered defaults: two entries for one key in a single TOML table is a
  parse error, not a last-wins override, so a client must be able to restate
  every key it wants. Pristine content = exactly what a non-placeholder build
  renders, plus a marker/explanation comment and `#` padding — comments don't
  parse, so the effective env still equals the baked defaults, which is what
  keeps "pristine implies no fresh intent" true.
- **A real `[env]` header is emitted even with no values.** Today a valueless
  build renders the commented-out example (`# [env]`); injected `KEY = "value"`
  lines under that would land in the root table, not `[env]`.
- **Manifest:** a new top-level `"env": {size, sha256, ranges}` key alongside
  `placeholders[]` — NOT a pseudo-entry in `placeholders[]`, whose `path` is
  documented as a real FAT-root path and whose `size` is the whole file's size.
  `gosd_inject` stays `1`: the key is additive and optional.
- **Sub-range plumbing:** `image.Spec.ReportRanges` becomes a `[]RangeRequest`
  (`{Path, OffsetBytes, LengthBytes}`, zero length = whole file) so the image
  layer clips to the requested span exactly where it already clips to file
  size. `FileRanges["gosd.toml"]` then holds precisely the env body's ranges
  and no span has to escape `pipeline.Assemble`.
- **Client contract:** overwrite the ranges with exactly `size` bytes of valid
  `[env]` body — `KEY = "value"` lines and comments, no section headers (a
  header would swallow the rest of the file), padded with newlines.

### Known consequence to document, not solve here

A snapshotted env value round-trips through `/data` AND is re-rendered into the
next card's `gosd.toml`. That is already true of WiFi passphrases and tunnel
tokens, but it means an injected secret survives a reflash someone may have
intended as a wipe. Whether injected keys need an ephemeral opt-out is a real
question — record it, don't invent a mechanism inside this bean.

Recovery of ANY kind still presupposes `/data` survived: an
`--data-size=expand` image whose on-card ABI (`--boot-size`,
`--data-filesystem`, `--label-prefix`) is unchanged.

## Todos

- [x] `internal/gosdtoml`: pad the `[env]` body to a requested size, report its
      span, real `[env]` header when reserving, deterministic render
- [x] `internal/image`: `ReportRanges` takes sub-span requests
- [x] `internal/inject`: manifest `env` key, region hashing, validation
- [x] `internal/pipeline` + `cmd/gosd`: `--env-placeholder` end to end
- [x] Tests incl. an end-to-end build -> patch the region -> read back ->
      parse -> assert the merged env, and a provsnapshot test pinning
      "injected value blocks the restore, pristine region does not"
- [x] Docs: image-injection, gosd.toml, runtime (incl. the reflash story the
      v0.4.2 section omits)
- [x] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
      `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`


## Summary of Changes

- **`gosd build --env-placeholder <size>`** reserves exactly that many bytes
  for gosd.toml's `[env]` body and publishes the region in
  `<image>.inject.json` under a new top-level `env` key
  (`{size, sha256, ranges}`); `gosd_inject` stays `1`, so the key is purely
  additive for existing clients. Sizes are refused up front above a 1MiB
  ceiling (`maxEnvPlaceholderBytes` — a units slip, not a real request) and,
  once the body is rendered, when the values baked in don't fit.
- **`internal/gosdtoml`:** `RenderWithReservedEnv` renders the same file as
  `Render` with the `[env]` body padded to the reserved size and its `Span`
  reported. The pristine region is the marker comment (`# GOSD-INJECTABLE v1
  env`), then exactly the values `Render` would have written, then `#`
  padding — all comment around the values, so the region PARSES to the baked
  defaults and a pristine card carries no "fresh intent". A live `[env]`
  header is emitted even with no values, because injected keys under the
  commented-out example would land in the root table.
- **`internal/image`:** `Spec.ReportRanges` is now `[]RangeRequest`
  (`{Path, OffsetBytes, LengthBytes}`, zero length = whole file), and
  `spanOfRanges` narrows a file's reported ranges to the requested span
  where the clipping already happened. Duplicate paths and negative bounds
  are refused before any image bytes exist. No span escapes
  `pipeline.Assemble`: `FileRanges["gosd.toml"]` already holds exactly the
  region.
- **`internal/inject`:** `WriteManifest` takes a `ManifestSpec` (board,
  placeholders, `EnvReservedBytes`, file ranges) rather than a growing
  parameter list. The region's hash is read back from the bytes actually
  written at the ranges being published, so a wrong range makes the
  published hash wrong too — which is exactly what a client's pristine check
  catches.
- **Recovery semantics, which is the point of the whole design:** an injected
  value is an ordinary gosd.toml value, so `provsnapshot` needed no change at
  all. Two tests pin it end to end from real rendered bytes — a pristine
  reserved region still lets the snapshot restore the operator's values after
  a plain reflash, and an injected one blocks that restore so a re-injection
  is never shadowed by the previous device's value.
- **Tests:** the `cmd/gosd` acceptance test builds an image, verifies the
  published hash, patches the region with a plain `os.WriteAt`, and reads
  gosd.toml back at the FAT level to assert the `[env]` table now parses to
  the injected settings AND that the hostname elsewhere in the file is
  untouched. Plus gosdtoml unit tests (exact size, parse-equivalence with an
  unreserved build, live header, too-small refusal, determinism) and
  `spanOfRanges` units including the fragmented case that real FAT
  allocation never produces.
- **Docs:** `docs/image-injection.md`'s environment-variable section is
  rewritten around the flag — what to write into the region, what happens on
  the next reflash (the per-key rules and the `/data` precondition), the
  secrets consequence (a snapshotted secret outlives a reflash; only clearing
  `/data` removes it), and a pointer to the placeholder-file route for
  settings that aren't environment variables. Manifest schema, client
  algorithm, `docs/gosd.toml.md`, `docs/runtime.md`, COMPATIBILITY.md and
  `docs/releases/UNRELEASED.md` updated to match.
- **Not in this PR:** npm-package support (bean gosd-ypyz). A browser client
  can splice the region today from the manifest alone; `withPlaceholders`
  doesn't yet know the `env` key.
