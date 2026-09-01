---
# gosd-sqtb
title: internal/wifictl protocol + public wifi/ package (blocking Join)
status: todo
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:28:29Z
parent: gosd-ojbm
---

Part of the runtime-WiFi-join epic — read the epic's locked decisions first; decisions 1, 2 and 7 govern this bean.

Deliver two packages, no gosd-init changes:

## `internal/wifictl` — the request/status file protocol

Shared by the public package (this bean) and the wifiup reconciler (sibling bean) so the two sides cannot drift.

- `const Dir = "/run/gosd/wifi"`, request at `request.json`, status at `status.json` (paths derived from a Dir parameter/variable so tests and the public package's device_other path can point elsewhere).
- `Request{ID, SSID, Passphrase string, Persist bool}`; `Status{ID, State, Error string}` with states `joining`, `joined`, `failed` (terminal: joined/failed).
- Atomic write helper: temp file in the same dir + rename, file 0600, dir created 0700. /run is tmpfs — atomicity is for torn-read avoidance, no fsync/crash-ordering argument needed (unlike on-card commits).
- Read helpers must distinguish "file absent" (normal) from "unparseable" (return error; the reconciler will decide policy). Mirror the shape of `internal/faultdrop`/`internal/secretreg` where it fits — look there before writing anything new.

## `wifi/` — public, app-facing (semver-relevant)

- `Join(ctx context.Context, creds Credentials, opts Options) error`. Credentials{SSID, Passphrase}; Options{Persist bool}. Docstrings throughout — this is public API documentation.
- Behavior: validate (SSID non-empty; empty Passphrase = open network, allowed per fleet WiFi scope), call `fault.RegisterSecretString(passphrase)` first (epic decision 7, skip for empty), write the request with a unique id (crypto/rand hex is fine), then poll `status.json` (~500ms) until the status carries this request's id and a terminal state. joined → nil; failed → error including the reason verbatim. ctx done → ctx error (the wait is cancelled, not the attempt — say so in the docstring).
- Off-device: the **`gosd` build-tag axis exactly like `fault/`** (`device_gosd.go` / `device_other.go` — read fault/'s pair first and mirror it). Without the tag Join returns an immediate actionable error ("wifi.Join only works in an app built by gosd build; on a device it would have joined SSID …"-style, matching fault/'s honesty precedent). NOT `linux`/`!linux` — a plain `go test` on Linux CI must never touch a real /run.
- Tests: behavioral, fake-driven, macOS-passing — point the package at a temp dir, run a fake reconciler goroutine that writes status responses, assert Join's outcomes: success, failure reason surfaced, ctx cancellation, stale status with an old id ignored, off-device error. Keep them concise.

## Notes

- Branch `bean/gosd-sqtb-wifi-join-api` from main. PR titled for the public API; "Part of gosd-ojbm" in the body. Needs a `.changeset/*.md` change file (minor — new public package; format in docs/releasing.md).
- Run every quality gate in CLAUDE.md (both golangci-lint invocations included) before pushing; foreground `gh pr checks <n> --watch --interval 30` after.
- Do not merge the PR — JP reviews and merges.
