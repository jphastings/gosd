# GoSD — guide for implementing agents

GoSD turns a Go main package into flashable SD-card images for small ARM boards
(see COMPATIBILITY.md for the board × feature matrix). See README.md for the
pitch. All work is
planned and tracked in **beans** — run `beans prime` at the start of a session,
and pick up work with `beans list --ready`. Bean bodies contain locked
per-task decisions: follow them; do not relitigate them in code review or
redesign them mid-task. If a locked decision proves wrong in practice, stop and
say so in the bean rather than silently diverging.

## Workflow

- One bean = one branch = one PR. Branch name: `bean/<bean-id>-<short-slug>`.
- JP reviews and merges every PR. Do **not** self-merge, even on green CI.
- CI must be green before requesting review. Include the bean file changes
  (status, checked todos, Summary of Changes) in the same PR as the code.
- Commit messages: imperative subject, body explains why. No test-result
  summaries in commit messages.
- `beans update` applies only the LAST `--body-replace-old/--body-replace-new`
  pair per invocation (the GraphQL path differs). Do one replacement per call,
  and check off todos one at a time.
- `beans create --json` returns the new id at `.bean.id`, NOT `.id` —
  `jq -r .id` silently yields `null`, which then cascades into confusing
  "parent bean not found: null" errors.
- Stacked work: when a task depends on an as-yet-unmerged PR, branch from that
  PR's branch (not `main`), say "stacked on #NN" in the body, and rebase onto
  `main` once it lands. Keep stacks shallow — prefer waiting for a merge over
  towering unreviewed PRs.

## Project-wide locked decisions

- **Module path:** `github.com/jphastings/gosd`. **License:** MIT (LICENSE file).
- **Language:** pure Go everywhere; `CGO_ENABLED=0`. No build step may require
  root, Docker, or Linux — `go test ./...` must pass on macOS and Linux.
  Linux-only runtime code goes behind build tags. **Carve-out:** `gosd build`
  itself never requires Docker; `gosd build-kernel` (opt-in custom kernel
  compiles, see `docs/custom-kernels.md`) and `gosd build-external` (opt-in
  companion-binary cross-compiles, see `docs/externals.md`) each require
  Docker or Podman, by design, and say so in their own `--help` text and
  errors.
- **Target:** per-board architecture, all `GOOS=linux`: `GOARCH=arm64` for
  pi-zero-2w / radxa-zero-3e / nanopi-zero2 / qemu-virt, and `GOARCH=arm
  GOARM=6` for pi-zero-w (BCM2835 is armv6, 32-bit only). The build pipeline
  compiles the app and gosd-init once per architecture needed by the selected
  boards (decided 2026-07-06; was arm64-only).
- **Board IDs:** `pi-zero-2w`, `pi-zero-w` (epic gosd-ajpz),
  `radxa-zero-3e`, `nanopi-zero2` (FriendlyElec RK3528A — epic gosd-cwjf);
  also `qemu-virt` (internal —
  see the "qemu-virt board" decision below: registered and buildable via
  explicit `--board=qemu-virt`, but excluded from `--help` text, the default
  build set, and catalog generation). `gosd build` with no `--board`
  builds **all** (public) boards, emitting `<appname>-<board>.img` next to
  each other; `--board` (repeatable) restricts.
- **Naming surfaces:** env vars `GOSD_*`; kernel cmdline params `gosd.*`;
  FAT partition labels `GOSD-BOOT` / `GOSD-DATA`; boot-partition config file
  `gosd.toml`; app build tags `gosd_<board-id>` (underscored, e.g.
  `gosd_pi_zero_2w`), passed to the app compile only (see
  `boards.BuildTag` and `docs/board-build-tags.md`).
- **Default hostname:** the sanitized basename of the app's main package,
  overridable via `--hostname` and `gosd.toml`.
- **Public API surface** (semver-relevant): `cmd/gosd`, `gadget/` (USB gadget
  library, v0.3), `device/` (app-facing runtime helpers, v0.3). Everything else
  lives under `internal/`.
- **gosd-init source location:** `gosd build` builds gosd-init from a local
  checkout when one's found (current directory's module, or the checkout gosd
  itself was compiled from), otherwise from `github.com/jphastings/gosd` at
  gosd's own build version via `go mod download`; `--gosd-init-src <dir>` is
  the escape hatch. See `internal/build/gosdinit.go`.
- **Third-party binary blobs** (Pi GPU firmware, WiFi firmware, Rockchip rkbin)
  are never re-hosted in our releases: the CLI downloads them from upstream at
  pinned URL + sha256 (per-board `manifest.json`) and caches them. Our artifact
  releases (`artifacts/vX.Y.Z` tags) contain only what we compile — kernels and
  U-Boot — with source repo, commit, and config recorded in the manifest (GPL
  compliance). CLI releases are plain `vX.Y.Z` tags and pin an artifact version.
  Developers never *have to* compile a kernel themselves — `gosd build-kernel`
  (epic gosd-47rm) is an opt-in path for compiling in a driver GoSD's stock,
  trimmed kernels cut; see `docs/custom-kernels.md`.
- **End-user flashing path (decided 2026-07-05):** the flagship flow is a
  Raspberry Pi Imager custom-repository catalog entry — `gosd build` can emit
  an `os_list.json` entry declaring `init_format: "cloudinit"`, the developer
  hosts it next to their image, and end users paste the repo URL into Imager's
  Settings → Custom repository to get the full WiFi/hostname wizard.
  `gosd.toml` hand-editing is the always-present fallback (works with any
  flasher). Consequence: gosd-init's provisioning parser reads cloud-init
  YAML + gosd.toml only; `firstrun.sh` parsing is out of scope (log-and-point
  -at-gosd.toml if encountered). See docs/provisioning-formats.md.
- **qemu-virt board:** an internal-only board profile for CI and local
  testing (`qemu-system-aarch64 -M virt`, virtio, SD appears as /dev/vda).
  It is EXCLUDED from default all-boards builds and from end-user docs;
  build it only via an explicit `--board=qemu-virt`.
- **gosd-init has no interactive surface**: no shell, no SSH, no remote debug,
  ever. Serial console output and app logs only. The only network listeners in
  gosd-init are mDNS (and, later, the explicitly-designed update endpoint).
- **WiFi scope:** WPA2-PSK and open networks only through v0.x. WPA3/EAP are
  out of scope — log clearly when encountered.
- **Supported CLI hosts:** macOS and Linux (amd64/arm64), enforced by CI.
  Windows is untested best-effort; don't break it gratuitously.

## Board work & artifact releases

- **Kernel-build source of truth is `internal/kernelspec`** (a declarative
  Go `KernelSpec` per board), not shell scripts — `gosd build-kernel`
  (`internal/kernelbuild`) reads it directly. Change a board's kernel build
  there, not by hand-editing a retired `build.sh`/`docker-build.sh`.
- **Building a board's kernel needs the board *registered*, not just a
  `kernelspec` entry.** `gosd build-kernel --board <id>` resolves `<id>`
  through `internal/boards` (registered in `cmd/gosd/build.go`) *before*
  looking up its `kernelspec` entry, so a kernelspec entry with no registered
  board fails with "unknown board". A new board's kernel therefore isn't
  buildable until its board profile is registered — `RegisterInternal` is
  enough (keeps it out of default all-boards builds, like qemu-virt), so the
  board-profile bean's registration is a de-facto prerequisite of the kernel
  bean's build even when the plan sequences them the other way. Adding a
  `kernelspec` entry also means updating the board-enumerating test lists in
  `internal/kernelspec/kernelspec_test.go` (the board-count list, the Rockchip
  DTS-patch allowlist, and the kernelspec-outputs-vs-Artifacts board map).
- **`gosd build-kernel` builds are content-addressed and cached** (kernel ref
  + image digest + fragment/patches + overlay) in a durable per-OS state dir
  (`internal/kernelbuild`'s `defaultBuildRoot`): identical re-runs are instant
  cache hits, so never re-run a long build just to re-emit its artifacts.
  Container bind mounts must stage under the user's home — macOS's temp dir
  (`/var/folders`) isn't shared with the Docker/colima VM and
  `~/Library/Caches` is storage-pressure-evictable; each silently killed a
  real 75-minute build (beans gosd-0p21, gosd-l4y9). colima in its default
  docker-runtime mode is a fully supported daemon (it presents as a normal
  docker context). Because those mounts are local paths, the build must run
  where the docker daemon and the repo live *together*: a remote/SSH docker
  context driven from your laptop mounts empty dirs and fails at once. To use
  a beefier build box, clone the repo under *its* `$HOME` and run
  `gosd build-kernel` there against its own local docker (mind the same
  home-dir mount rule if that box is also a Mac running colima).
- **Artifact releases are tag-first, bump-second.** Any change under
  `build/boards/*` that alters a compiled artifact (kernel config/fragment,
  DTS patch, U-Boot) only reaches real (non-`--artifacts-dir`) builds after a
  new `artifacts/vX.Y.Z` GitHub release. Ship the build change WITHOUT bumping
  `internal/artifacts.Version` in the same PR — bumping to an unpublished tag
  turns the qemu boot-to-HTTP CI job red. JP pushes the tag; then a separate
  follow-up PR bumps `Version` and verifies against the real release. Full
  procedure in `docs/artifacts.md`.
- **Verify an artifact bump three ways, recorded in the bean:** clean-machine
  build (fresh `HOME`, no `--board`/`--artifacts-dir` → all public images from
  a real download), offline re-run (dead proxy → succeeds entirely from cache),
  and a content spot-check that the released artifact carries the change
  (e.g. `dtc -I dtb -O dts` shows the enabled node).
- **Peripheral enablement is per-SoC.** Pi boards: `dtparam=<x>=on` in the
  config.txt template (no artifact release needed). Rockchip boards (Radxa,
  NanoPi): a kernel-build DTS patch under `build/boards/<board>/kernel/patches/`
  that sets the bus `status="okay"` (plus a `spidev` child node with an
  accepted compatible for SPI) — NOT a runtime overlay, because our pinned
  U-Boots lack `OF_LIBFDT_OVERLAY`. Confirm each patch applies against the
  pinned kernel tag; a Rockchip DTS/config change triggers the release dance
  above.
- All boards pin the SAME kernel tag ("the fleet tag") — bump them together,
  never one board in isolation. Kernel/U-Boot Docker builds take 20-60 min:
  run them backgrounded and poll the log, never in a foreground shell.

## Quality gates — run ALL of these before every commit/PR

- `go test ./...`
- `go vet ./...`
- `gofmt -l .` (must print nothing)
- `golangci-lint run ./...` AND `GOOS=linux golangci-lint run ./...` — CI lints
  from Linux, so the second invocation is the one that must match CI; run both
  so darwin-only and linux-only files are each checked. A finding that is a
  cross-GOOS false positive (symbol only used in a `_linux.go` file) gets a
  `//nolint:<linter> // <reason>` comment, not an exclusion rule.
- If `golangci-lint` reports a finding referencing a path in a worktree that no
  longer exists, it's a stale-cache false positive from a removed sibling
  worktree: `golangci-lint cache clean` and re-run before believing it.

## Code conventions

- Errors shown to CLI users must be actionable ("X failed because Y; try Z"),
  never bare wrapped chains.
- Tests are behavioral and concise; fixture-driven where the bean says so.
- Comments only where code can't explain itself; docstrings on exported API.
- Board or feature status changes must update COMPATIBILITY.md in the same PR.
- gosd-init runtime code follows one shape: pure logic behind a small interface
  seam with fake-driven tests that pass on macOS; real syscalls isolated in
  `platform_linux.go` (`//go:build linux`) with `platform_other.go` stubs. New
  features (see `netup`, `wifiup`, `timesync`, `mdnsresponder`) mirror it.
- `gosd build` behaviour gets a fixture-driven integration test that reads the
  built image back and asserts contents, with a network-tripwire RoundTripper
  proving no fetch happened (pattern in `cmd/gosd/build_integration_test.go`).
- Examples stay stdlib-only where practical and must cross-compile for every
  board arch (arm64 AND `GOARCH=arm GOARM=6`); peripheral examples degrade
  gracefully when the device/bus is absent (see `examples/i2cscan`).
  `examples/sattrack` is the reference for a bigger example: third-party deps
  when its bean justifies them, an in-tree `gosd build-kernel` recipe
  (`examples/sattrack/kernel/`), and graceful no-display degradation.
