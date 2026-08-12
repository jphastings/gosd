---
# gosd-5pz2
title: Document injecting app env vars from a web download
status: completed
type: task
priority: normal
created_at: 2026-08-12T10:25:17Z
updated_at: 2026-08-12T10:33:50Z
---

`docs/image-injection.md` describes the placeholder/manifest contract but never
answers the obvious follow-up question: "how do I inject environment variables?"
(asked by JP, 2026-08-12). The answer is non-trivial and currently undiscoverable:
app env comes only from `config.json` (inside the compressed initramfs, no fixed
byte ranges) and `gosd.toml [env]` — and `gosd.toml` is refused as a
`--placeholder` path (`internal/pipeline`'s case-insensitive collision check), so
`[env]` cannot be spliced at all. The manifest's `ReportRanges` covers placeholder
paths only.

## What to document

A new "Injecting environment variables" section in `docs/image-injection.md`:

- State the limitation up front: `[env]` is not injectable; placeholders are
  separate files.
- The supported pattern: reserve a placeholder the app reads itself from `/boot`
  (mounted read-only, always present), parse it at startup, and `os.Setenv` so
  existing `os.Getenv` call sites are unchanged.
- A worked example: `--placeholder app.env=4KiB`, the `withPlaceholders` call, and
  the ~15-line Go loader (pristine-prefix check, JSON body, trailing-newline
  padding is what the npm package emits so the format must tolerate it).
- The two things this loses versus a real `[env]` value, both easy to miss:
  crash-report redaction is NOT automatic (gosd-init's `envRedactionRules` only
  covers merged `[env]`/baked values), so the app must call
  `fault.RegisterSecretString`; and the file is plaintext on the boot FAT, exactly
  like `gosd.toml`'s own `[env]`.

## Todos

- [x] Add the section to `docs/image-injection.md`
- [x] Verify the Go loader snippet compiles as written
- [x] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`

## Notes

Follow-up feature work — making env vars genuinely injectable — is tracked
separately; this bean documents the mechanism that exists today.


## Summary of Changes

- `docs/image-injection.md` gains an "Injecting environment variables" section:
  why `[env]` itself can't be spliced (`config.json` is inside the compressed
  initramfs; `gosd.toml` is refused as a `--placeholder` path; cloud-init carries
  hostname/WiFi only), then the pattern that does work — reserve a placeholder,
  read it from the read-only `/boot` mount at startup, `os.Setenv` it so existing
  `os.Getenv` call sites are untouched — with the `--placeholder app.env=4KiB`
  build flag, the `withPlaceholders` call, and a compile-checked Go loader
  (pristine-prefix check, JSON body so trailing-newline padding stays legal).
- Three caveats documented, each a thing that only bites later: crash-report
  redaction is not automatic for app-loaded values (call
  `fault.RegisterSecretString`, and only for the genuinely secret ones), nothing
  stops an injected key clobbering `GOSD_BOARD`/`GOSD_HOSTNAME` the way
  `--env` and gosd.toml are policed, and the file is plaintext on the boot FAT.
- `docs/runtime.md`'s app-environment section now points at that section for the
  per-device case, which neither baked defaults nor a hand-edited `gosd.toml`
  serve.
- Follow-up bean gosd-dwub proposes making env vars genuinely injectable (two
  designs, JP to choose) — no code written for it here.
