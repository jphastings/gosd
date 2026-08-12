---
# gosd-48k0
title: 'Injectable gosd.toml: reserve the whole config file, not just [env]'
status: completed
type: feature
priority: normal
created_at: 2026-08-12T21:02:33Z
updated_at: 2026-08-12T21:19:06Z
---

SUPERSEDES the `--env-placeholder` mechanism from bean gosd-dwub, before either
half of it ships: v0.5.0 was never tagged, and the npm 0.2.0 publish failed on
a tag/manifest mismatch, so `--env-placeholder` and `options.env` exist only on
main. Nothing is deprecated by this bean; the narrower flag is replaced.

## Why

Reserving the body of `[env]` reaches app settings and nothing else. JP needs
`[ingress.cloudflared]` / `[ingress.tailscale-funnel]` set per device at
download time too — a tunnel token is exactly the kind of per-user secret an
image can't bake.

Today's region can't carry a section: the contract forbids headers in it,
because one would capture every line gosd wrote below. It would *appear* to
work — `gosdtoml.Render` writes ingress last and `gosd build` never bakes real
ingress values (pipeline passes a zero `Ingress`), so everything after the
region is commented-out example text — but that is a coincidence of render
order, not a contract, and the first real section written below `[env]` would
break every already-published image's injection silently.

## Locked decisions (JP, 2026-08-12)

- **The reserved region is the WHOLE `gosd.toml`, comments and all.** Pristine
  content is exactly what a non-reserving build renders, padded to the
  reserved size. An image that nobody injects into is therefore unchanged in
  behaviour and still self-documenting.
- **The client is handed the pristine text and may edit or replace it.** The
  manifest publishes the region's exact padded bytes as text alongside its
  hash, so a caller can adapt the file it was actually given (keeping gosd's
  plain-language comments) rather than reconstructing a template the package
  would have to duplicate and keep in sync. Publishing it leaks nothing: the
  same bytes are already in the public `.img`, and the manifest is the trusted
  side of the threat model, so the client can verify the text it was handed
  against the `sha256` already published for the region.
- **`--env-placeholder` is replaced, not kept as an alias** (nothing shipped).
- **Nothing changes on the device.** The region is still just `gosd.toml`, so
  gosd-init parses it as always and `provsnapshot` treats every injected value
  as the operator's own intent — including the ingress sections, which it
  already restores as a WHOLE unit across a reflash precisely because there is
  no baked default for one to differ from (docs/runtime.md). Injected tunnel
  credentials survive an over-flash on the same argument `[env]` values do.
- **Flag:** `gosd build --config-placeholder[=<size>]`, defaulting to a
  generous reserve when given without a value (cobra `NoOptDefVal`); the
  template is a few KiB and the boot partition is 256MiB, so the size knob is
  for unusual cases, not the common path.
- **Manifest:** the `env` key becomes `config` — `{size, sha256, ranges,
  pristine}` — still additive, `gosd_inject` stays `1`.
- **Client API:** `options.config` accepts a complete `gosd.toml` string, or a
  function `(pristine: string) => string` for the adaptive case. `options.env`
  goes; `renderEnvBody` stays exported as a helper (its `GOSD_*` and
  key-shape refusals are worth keeping for callers assembling an `[env]`
  table by hand).

## Todos

- [x] `internal/gosdtoml`: pad the whole rendered file to a reserved size,
      replacing the `[env]`-body span
- [x] `internal/inject` + `internal/pipeline` + `cmd/gosd`:
      `--config-placeholder`, manifest `config` key carrying the pristine text
- [x] Decide whether `image.Spec.ReportRanges`'s sub-span support still earns
      its keep once the region is a whole file; drop it if nothing uses it
- [x] Tests: build -> read pristine from the manifest -> edit it -> splice ->
      read back and assert hostname/[env]/[ingress.*] all parse
- [x] Docs: image-injection, gosd.toml, runtime, COMPATIBILITY, UNRELEASED
- [x] npm package: `options.config` — done HERE, not split out: CI regenerates the fixture with the Go code and runs the TypeScript integration test against it, so the two halves can't land apart without a red build
- [x] Quality gates (Go and, in the npm PR, `js/`)


## Summary of Changes

- **`gosd build --config-placeholder[=<size>]`** replaces `--env-placeholder`
  (unreleased, so nothing is deprecated): it pads the card's whole gosd.toml
  to the reserved size — 16KiB when the flag is given with no value — and
  publishes it in `<image>.inject.json` under a top-level `config` key
  (`path`, `size`, `sha256`, `ranges`, `pristine`).
- **`pristine` is the file's exact text**, read back out of the finished image
  at the very ranges being published, so a wrong range makes the published
  hash AND text wrong together. That is what lets a client edit the config it
  was handed instead of reconstructing gosd's template in TypeScript and
  keeping it in sync forever.
- **`internal/gosdtoml`:** `RenderReserved` renders exactly what `Render`
  does, plus a short trailer explaining the reserved space and `#` padding to
  size. The `[env]`-body span, its marker and the live-header special case
  are gone — the client writes whole files now, so it can write its own
  headers.
- **`internal/image`:** `Spec.ReportRanges` is back to `[]string`.
  `RangeRequest`/`spanOfRanges` existed only to carve the `[env]` body out of
  gosd.toml; with the region being the whole file, they were dead generality.
- **Client (`@jphastings/gosd`):** `options.env` becomes `options.config`,
  taking either a replacement string or `(pristine) => string`. The region
  view added for `[env]` carried straight over — the config file is keyed by
  its own path, so substitution, resume and overlap-checking needed no
  further change. `renderEnvBody` stays exported for callers assembling an
  `[env]` table by hand, since its escaping and `GOSD_*` refusal are the
  fiddly parts.
- **Proof it reaches the thing this bean exists for:** both the Go acceptance
  test and the cross-implementation integration test edit the commented-out
  `[ingress.cloudflared]` block in the published pristine text into a real
  tunnel, splice it, and assert the device-side parse yields the token — with
  the baked `[env]` default and gosd's own comments still in place, and every
  byte outside the region unchanged.
- **Docs:** the image-injection guide's environment-variable section is now
  "Injecting configuration: hostname, WiFi, settings, ingress", covering what
  to write, edit-vs-replace, the per-key reflash rules, the `/data`
  precondition and the secrets consequence. gosd.toml, runtime,
  COMPATIBILITY, UNRELEASED and the npm README follow.
