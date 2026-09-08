---
# gosd-42vb
title: 'ready package: hold the status LED on "booting" until the app signals ready'
status: in-progress
type: feature
created_at: 2026-09-08T12:22:43Z
updated_at: 2026-09-08T12:22:43Z
---

An app that needs WiFi (or anything else) before it is genuinely "all okay" currently cannot stop the status LED showing "running": gosd-init flips the LED the instant `/app`'s process starts (`Supervisor.Start` in `cmd/gosd-init/internal/boot/sequence.go`), before `main` has run a line, and networking never blocks that (a locked decision — see "Networking comes up after your app does" in docs/runtime.md). This bean lets an app hold the LED in the "booting" flash until it has done its own checks and says so.

There are still exactly three LED states (docs/status-led.md). This changes only WHEN the booting→running transition happens, never what the states look like.

## Locked decisions (JP, 2026-09-08)

1. **Importing the package IS the declaration — no build flag.** `gosd build` runs `go list -deps` on the app's main package, under the same env and `-tags` as that board's app compile (`archEnv` + `boards.BuildTags`, so a `//go:build gosd` file that imports it counts), and sets a new `appSignalsReady: true` in `config.json` when `github.com/jphastings/gosd/ready` is in the dependency graph. Per board, at the point `internal/pipeline` assembles `initcfg.Config` (and the `gosd run` path if it assembles its own). Add a small helper in `internal/build` next to `mainPackageName` (e.g. `ImportsPackage(pkgPath string, opts AppCompileOptions, arch boards.Arch, importPath string) (bool, error)`); a `go list` failure there is a hard build error through `explainBuildFailure`, same as the existing inspection. JP chose inference over a flag so there is one fewer thing for a developer to remember; the cost — an accidental import that never calls the function holds the LED forever — is paid for by decision 4's serial line.
2. **Public package `ready`** (semver-relevant, docstrings throughout) with one function, `func Signal() error`, that creates the empty marker file `/run/gosd/ready`. Same file-drop idiom as `fault`/`wifi` (no socket; gosd-init polls a tmpfs it already mounts). Same `gosd` build-tag split as `fault/device_gosd.go` / `device_other.go` — read that pair first and mirror it; NOT linux/!linux. Off the tag, `Signal` is a silent no-op returning nil and touches no filesystem. Calling it more than once is harmless. Put the marker's path in a tiny shared `internal/readymark` (Dir under `faultdrop.Dir`, a `Path`/`Mark`/`Marked` trio or similar) so the writer and gosd-init's reader cannot drift — the `internal/wifictl` precedent.
3. **gosd-init holds, then polls.** When `cfg.AppSignalsReady`, the `Supervisor.Start` hook does NOT set `Running` on the first successful start; instead it starts ONE guarded goroutine (`guard.Go`, like networking — a panic in PID 1 is a dead appliance) that polls for the marker (~500ms, the tsfunnel network-up poll style; go through a `Deps` seam so the boot-sequence tests drive it on macOS) and sets `Running` exactly once, then exits. Keep the existing one-time `appHandedOver` rule: a crash-and-restart after the LED went running never returns it to booting; a crash BEFORE ready leaves the poller waiting and the restarted instance's `Signal` flips it. `fault.Fatal` before ready goes solid as today.
4. **Serial line, at first app start, when holding:** `status LED: holding "booting" until the app calls ready.Signal() — it imports github.com/jphastings/gosd/ready, so the LED only shows "running" once that call is made`. And when the marker appears: `status LED: app signalled ready`. This line is the whole mitigation for "imported it, never called it" — gosd-init has no other surface.
5. **No readiness timeout, no fourth state.** An LED stuck on "booting" is the honest signal; flipping anyway after N minutes would make it a lie. An app that wants solid-on plus an explanation on the card calls `fault.Fatal`. Holding the ingress agents on readiness is out of scope.

## Todo

- [x] `internal/readymark`: shared path + tiny write/check helpers, tests
- [x] `ready/`: `Signal()` with the gosd/!gosd split; package doc explaining the import-as-declaration rule and the never-called consequence; off-device no-op test
- [x] `internal/build`: dependency-graph inspection helper + test (testdata packages: one importing `ready`, one not; assert tags are honoured)
- [x] `internal/initcfg.Config.AppSignalsReady` (`json:"appSignalsReady,omitempty"`, docstring), set by `internal/pipeline` per board; `gosd run` path if it builds its own config
- [x] gosd-init boot sequence: hold + poll + both log lines; tests with the fake LED asserting no `Running` at Start when set, exactly one after the marker appears, and the hold line logged
- [x] `cmd/gosd` fixture-driven integration test reading `config.json` back from a built image for an app that imports `ready` (pattern in `cmd/gosd/build_integration_test.go`)
- [x] docs: `docs/status-led.md` (the "Running" row's "When", plus a short section) and `docs/runtime.md` (a pointer from "Networking comes up after your app does" to it); `.changeset/*.md` (minor: new public package + config.json field)
- [x] Every quality gate in CLAUDE.md (both golangci-lint invocations); PR; foreground `gh pr checks <n> --watch --interval 30`

## Notes

- Branch `bean/<id>-app-ready-signal` from main. Do not merge the PR — JP reviews and merges.
- The `docs/status-led.md` prose is a public promise ("nothing in your app needs to change") — keep it true: the default path is untouched; only an app that imports `ready` opts in.

## Summary of Changes

- `internal/readymark`: `Dir`/`Path` constants plus `Mark`/`Marked`, the
  shared marker-file contract between the `ready` package and gosd-init's
  boot sequence (mirrors `internal/wifictl`'s precedent).
- `ready/`: new public package, `Signal() error`, with the same `gosd` /
  `!gosd` build-tag split as `fault` (`device_gosd.go`/`device_other.go`).
  Package doc covers the import-as-declaration rule and the
  never-called-it consequence.
- `internal/build.ImportsPackage(pkgPath, opts, arch, importPath)`: runs
  `go list -deps` under `archEnv(arch)` and `opts.Tags`, next to
  `mainPackageName`; testdata fixtures cover an unconditional import, no
  import, and a tag-gated import (proving `-tags` reaches `go list`).
- `internal/initcfg.Config.AppSignalsReady` (`omitempty`), wired through
  `pipeline.Options.AppSignalsReady` into `config.json`; both `cmd/gosd
  build` (per board, reusing that board's own compiled `AppCompileOptions`
  via a new `archBinaries.appOpts` field) and `cmd/gosd run` call
  `build.ImportsPackage` to set it.
- `cmd/gosd-init/internal/boot/sequence.go`: when `cfg.AppSignalsReady`,
  `Supervisor.Start` no longer flips the LED to Running on the first
  successful start; it logs the holding line once and launches exactly one
  guarded poller (`waitForAppReady`, `DefaultAppReadyPollInterval` =
  500ms, via the new `Deps.AppReady` seam) that sets Running and logs the
  signalled line the first time the marker appears. `appHandedOver` became
  an `atomic.Bool` since the poller and `Start` both write it now.
  `cmd/gosd-init/main.go` wires `Deps.AppReady` to
  `readymark.Marked(readymark.Dir)`.
- `cmd/gosd-init/internal/boot/logger.go`: added a mutex to `Logger.Printf`
  — a second goroutine now logs concurrently with the main boot sequence
  (the ready poller, alongside the pre-existing `StartNetworking`
  goroutine), and `go test -race` caught the pre-existing unsynchronized
  write this exposed. Fixing it in `Logger` covers every concurrent
  caller, not just the new one.
- Docs: `docs/status-led.md` gained a "Holding booting until your app is
  genuinely ready" section and updated table rows; `docs/runtime.md`
  points from its networking section at the new section.
- `.changeset/ready-package.md` (minor).

No deviation from the locked decisions. One interpretation: decision 3
says the poller should be "a `Deps` seam so the boot-sequence tests drive
it on macOS" — implemented as `Deps.AppReady func() bool`, checked via
`waitForAppReady`, mirroring `Deps.StartNetworking`'s existing shape
rather than adding a `readymark` import to the `boot` package itself
(kept out of `sequence.go` entirely; only `main.go` imports it).
