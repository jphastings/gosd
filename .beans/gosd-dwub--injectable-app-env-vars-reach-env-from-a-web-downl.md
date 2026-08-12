---
# gosd-dwub
title: 'Injectable app env vars: reach [env] from a web-downloaded image'
status: todo
type: feature
created_at: 2026-08-12T10:25:48Z
updated_at: 2026-08-12T10:25:48Z
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

- [ ] JP picks Option A or B (or rejects both); record the decision here before coding
- [ ] Implement the chosen design
- [ ] Tests: end-to-end build -> patch ranges -> boot-sequence env merge (fake-driven, macOS-safe), pristine-placeholder-is-absent, `GOSD_*` refusal still applies to injected keys, redaction rules cover injected values
- [ ] Docs: fold into the image-injection and gosd.toml/runtime env docs; update the workaround section gosd-5pz2 adds
- [ ] npm package support if the chosen design needs it (`js/` gates: format/lint/typecheck/build/test/test:integration)
- [ ] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`
