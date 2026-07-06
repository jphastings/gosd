---
# gosd-wnsj
title: 'gosd run: build + boot a qemu-virt image in one command'
status: completed
type: feature
priority: normal
created_at: 2026-07-05T15:24:35Z
updated_at: 2026-07-06T15:31:01Z
---

Follow-up to gosd-27lz (scripts/qemu-run.sh). Wrap the build-extract-boot flow in a first-class 'gosd run <pkg>' command: cross-compile, assemble a qemu-virt image, extract Image/initramfs from the FAT partition (internal/cmd/imgextract logic, callable directly rather than via go run), and exec qemu-system-aarch64 with serial on stdio and hostfwd 8080->80. scripts/qemu-run.sh proved the invocation; this promotes it from a repo-local dev script to something app developers get from the installed CLI. Needs a decision on qemu-binary discovery/version floor and on flag surface (port mapping, memory, extra qemu args).


## Decisions made (were open in the bean)

- **qemu-binary discovery:** `exec.LookPath("qemu-system-aarch64")` on PATH, no minimum version check. Every qemu-system-aarch64 exercised so far (Homebrew on macOS, apt's qemu-system-arm 7.2.x in CI, and 11.0.2 on this dev machine) supports the invocation, so there's nothing yet worth gating on. `gosd run` checks this *before* cross-compiling or assembling anything, so a missing qemu fails in milliseconds with an actionable, per-OS install hint (`brew install qemu` / `apt-get install qemu-system-arm`).
- **Flag surface:** `--port` (default 8080, hostfwd target), `--memory` (default 512 MiB), `--qemu-arg` (repeatable escape hatch appended verbatim), `--keep` (skip deleting the temp build dir + image after qemu exits, printing its path instead), plus `--hostname`, `--artifacts-dir`, `--gosd-init-src` passed straight through to the same build pipeline `gosd build` uses. Deliberately omitted: `--board` (fixed to qemu-virt - nothing to select), `--wifi-ssid`/`--wifi-pass` (qemu-virt has no WiFi hardware, only virtio-net), `--usb-gadget`/`--data-size` (not useful for the qemu inner dev loop; can be added later if wanted).

## Summary of Changes

- `internal/qemurun`: new package holding the one qemu-virt invocation gosd knows about - `ExtractBootFiles` (the FAT-partition extraction, moved from `internal/cmd/imgextract`), `Args` (builds the exact qemu-system-aarch64 flags validated by gosd-5wm0/gosd-27lz), `CheckAvailable` (actionable missing-binary error), and `Run` (extracts to a temp dir, execs qemu with stdio wired through, and tears down cleanly on context cancellation - SIGTERM first, killed after a 5s WaitDelay). Both `gosd run` and the CI/local runner script now share this one implementation instead of each keeping their own copy of the qemu flags.
- `cmd/gosd/run.go`: new `gosd run <path-to-main-package>` command. Fails fast on a missing qemu-system-aarch64 before doing any work; otherwise cross-compiles the app + gosd-init, assembles a qemu-virt image into a temp directory (same pipeline, artifact cache, and `--artifacts-dir`/`--gosd-init-src` escape hatches as `gosd build`), then hands it to `internal/qemurun.Run`. Ctrl-C (SIGINT/SIGTERM via `signal.NotifyContext`) stops qemu and deletes the temp directory; `--keep` preserves it and prints its path instead.
- `internal/cmd/imgextract`: now a thin wrapper around `qemurun.ExtractBootFiles` - no behavior change, just de-duplicated.
- `internal/cmd/qemuboot` (new): boots an already-built image via `qemurun.Run`, with its own SIGINT/SIGTERM handling. `scripts/qemu-run.sh` is now a thin wrapper around it (`exec go run ./internal/cmd/qemuboot <img>`), so the script, CI's qemu-boot job, and `gosd run` all boot qemu the exact same way. This incidentally fixes a latent leak in the original script: its `trap ... EXIT` cleanup for the extracted-files temp dir was registered before a final `exec qemu-system-aarch64`, which replaces the shell's process image and silently discards the trap, so every prior `qemu-run.sh` invocation leaked its extraction tempdir. `qemurun.Run` doesn't exec-replace itself, so its `defer os.RemoveAll(workDir)` always runs.
- `README.md`, `docs/runtime.md`: `gosd run` documented as the primary no-hardware dev loop; `scripts/qemu-run.sh` kept documented as the secondary "boot an already-built image" path.
- Tests: `internal/qemurun/qemurun_test.go` (Args's flag construction and defaults, ExtractBootFiles round-tripping a fixture image built via `internal/image.Write`, actionable errors for a missing image path / a non-qemu-virt image / a missing qemu binary - the qemu-invoking cases skip when qemu-system-aarch64 isn't on PATH, since the plain `test` CI job doesn't install it); `cmd/gosd/run_integration_test.go` (fails fast with no qemu on PATH; builds a real qemu-virt image from fake artifacts and hands it to a fake `qemu-system-aarch64` stand-in, asserting the invocation's flags; `--keep` preserves the built image and reports its path).

### Local end-to-end verification (this machine, arm64 macOS, real published artifacts/v0.1.0)

`go run ./cmd/gosd run ./examples/hello` (and the equivalent built-binary invocation) against the real, published kernel release:

- Cross-compile + assemble (cached kernel, no network): **~6s**.
- Guest boot to first successful HTTP response: **~3s** (app-reported uptime ~0.3-0.9s at first successful curl, consistent across runs).
- Total wall-clock from invocation to `curl http://localhost:8080/` succeeding: **~9s**, matching `host=<hostname> ... boots=N` responses from `examples/hello`.
- Ctrl-C (SIGINT) during an interactive run cleanly kills qemu and removes the temp build dir; confirmed empty of leftover `gosd-run-*`/`gosd-qemurun-*` directories and no lingering `qemu-system-aarch64`/`gosd` processes afterward. `--keep` was verified separately to leave the temp dir (with the built image) in place and print its path.
- `scripts/qemu-run.sh` re-verified against a real `--board=qemu-virt` image after the internal/qemurun refactor: boot-to-HTTP in ~3-5s; a real terminal Ctrl-C (SIGINT to the whole foreground process group, since qemu-run.sh/go run/qemuboot/qemu-system-aarch64 all share one pgid) tears everything down with no leftover processes or temp dirs; CI's exact teardown sequence (`kill $pid` then unconditional `pkill -f qemu-system-aarch64`) was also reproduced manually and leaves the host equally clean.

### Deviations from a strict reading of the bean

- The bean only asked for `gosd run` itself, calling out `internal/cmd/imgextract`'s extraction logic as something to make callable directly. Went further and also refactored `scripts/qemu-run.sh`/CI's qemu-boot job's underlying boot logic (via new `internal/cmd/qemuboot`) onto the same `internal/qemurun` package, so the qemu invocation itself has exactly one implementation instead of drifting between a Go command and a bash script. `scripts/qemu-run.sh`'s own CLI contract (`scripts/qemu-run.sh <path-to-image.img>`) and CI's ci.yml usage of it are unchanged.
