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
- `beans create` takes the title as a POSITIONAL argument
  (`beans create "Title" -t bug`) — there is no `--title` flag.
- `beans create --json` returns the new id at `.bean.id`, NOT `.id` —
  `jq -r .id` silently yields `null`, which then cascades into confusing
  "parent bean not found: null" errors.
- Stacked work: when a task depends on an as-yet-unmerged PR, branch from that
  PR's branch (not `main`), say "stacked on #NN" in the body, and rebase onto
  `main` once it lands. Keep stacks shallow — prefer waiting for a merge over
  towering unreviewed PRs. **Merging the base can silently strand a child**,
  in two different ways: merged as-is it lands on the stack branch rather than
  main (PR #98), and if the base branch is deleted at merge GitHub may CLOSE
  the child outright rather than retarget it, dropping its content with no
  conflict and no warning (PR #133 — the verified dtparam findings had to be
  cherry-picked onto main as #137). So after any stack merge: retarget
  survivors with `gh pr edit N --base main`, and verify the content actually
  reached main (`git show origin/main:<file>` or grep for it) rather than
  trusting a badge.
- **A CONFLICTING PR gets no CI at all.** GitHub can't create the test-merge
  commit, so `pull_request` workflows silently never trigger and
  `gh pr checks` reports "no checks reported" — which reads like an Actions
  outage but isn't (PRs #157 and #160 both lost time to this). When checks
  don't appear, check `gh pr view --json mergeable` FIRST; rebase onto main
  and CI starts on the push.
- Never open a PR against a repository outside the `jphastings` account
  without JP's explicit permission — upstream dependencies included. Prepare
  the patch in a local clone and record it in the bean instead; JP decides
  whether to send it.

## Project-wide locked decisions

- **Module path:** `github.com/jphastings/gosd`. **License:** MIT (LICENSE file).
- **Language:** pure Go everywhere; `CGO_ENABLED=0`. No build step may require
  root, Docker, or Linux — `go test ./...` must pass on macOS and Linux.
  Linux-only runtime code goes behind build tags. **Carve-out:** `gosd build`
  itself never requires Docker; `gosd build-kernel` (opt-in custom kernel
  compiles, see `docs/custom-kernels.md`) and `gosd build-external` (opt-in
  companion-binary cross-compiles, see `docs/externals.md`) each require
  Docker or Podman, by design, and say so in their own `--help` text and
  errors. **Carve-out:** `js/` (bean gosd-hcyn) is a separate pnpm-workspaces
  area — the `@jphastings/gosd` npm package (`@jphastings/gosd/downloads`, a
  browser/Node placeholder-substituting downloader for images built with
  `--placeholder`; bare `gosd` was 403'd by npm's typosquat guard, bean
  gosd-g7n4) — and is never part of any Go build or `go test ./...` run.
  TypeScript strict; build/test/lint/format via Vite+ (`vp`: tsdown library
  build, Vitest, oxlint, oxfmt); zero runtime dependencies,
  including a vendored streaming SHA-256
  (`js/packages/gosd/src/downloads/sha256.ts`) pinned by NIST CAVP vectors
  and `crypto.subtle.digest` cross-checks, since WebCrypto alone can't hash
  a stream incrementally. Its cross-implementation integration test's
  fixture generator, `internal/cmd/injectfixture`, IS Go module code and
  runs under the normal Go gates below. npm publishing is staged and
  tokenless (`npm/<directory>/vX.Y.Z` tag, keyed on the package's directory
  name under `js/packages/` — not its possibly-scoped npm name — → OIDC
  trusted publishing with provenance → the `next` dist-tag only; a human
  promotes to `latest`) — procedure in `js/PUBLISHING.md`; never publish
  from CI to `latest`.
- **Target:** per-board architecture, all `GOOS=linux`: `GOARCH=arm64` for
  pi-zero-2w / pi-3b / radxa-zero-3e / nanopi-zero2 / rock-4se / qemu-virt,
  and `GOARCH=arm GOARM=6` for pi-zero-w (BCM2835 is armv6, 32-bit only). The
  build pipeline compiles the app and gosd-init once per architecture needed
  by the selected boards (decided 2026-07-06; was arm64-only).
- **Board IDs:** `pi-zero-2w`, `pi-zero-w` (epic gosd-ajpz),
  `pi-3b` (BCM2837, one image covers the 3B and 3B+ — epic gosd-xhc3),
  `radxa-zero-3e`, `nanopi-zero2` (FriendlyElec RK3528A — epic gosd-cwjf),
  `rock-4se` (Radxa ROCK 4SE, RK3399-T — epic gosd-cuym); also `qemu-virt`
  (internal —
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
  library), `emmc/` (onboard-eMMC format/mount), `disk/` (the same for any
  attached mass storage — NVMe, USB drive, card reader) and `sound/` (ALSA PCM
  playback: `Open`/`OpenWith`, a `Device` to `Play` frames to; needs a
  `gosd build-kernel` kernel, see `docs/sound.md`). Everything else lives
  under `internal/`; `emmc` and `disk` share `internal/blockmount` (the
  format/mount orchestration, label rules and candidate selection) and
  `internal/diskfmt` (pure-Go inspect/format — FAT32 via go-diskfs, exFAT
  written directly from the Microsoft spec), so a change to one must not
  silently change the other's semantics. `emmc` is FAT32-only by design;
  exFAT is a `disk` option (bean `gosd-1ici`) and needs `CONFIG_EXFAT_FS`
  in the board's kernel, checked against `/proc/filesystems` before any
  write.
- **Layout ABI (decided 2026-07-31, docs/design/upgrade-path.md):** the boot
  volume size is per-app (`gosd build --boot-size`, default 256MiB) and is
  that app's on-card ABI — changing it in a later release erases GOSD-DATA
  on upgrade (cleanly, via the adoption gate) and is a release-notes-level
  breaking change. Nothing on-device may assume a fixed data-partition
  offset: derive it from the flashed MBR (partition 1 start + size), the way
  dataexpand does. Plain Imager reflash is the baseline upgrade path:
  `--data-size=expand` images keep GOSD-DATA via first-boot re-adoption, and
  the provisioning snapshot in /data self-heals gosd.toml hand-edits.
- **vfat `flush` is opt-in, default off (decided 2026-08-02, bean
  gosd-9m1k):** normal writeback everywhere (`gosd build --data-flush` /
  gosd.toml `data_flush` / env `GOSD_DATA_FLUSH` to opt in). Durability
  comes from docs/runtime.md's fsync sequence, never from `flush` — do not
  reintroduce it as a correctness measure.
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
- **`/data` durability is the app's choice (decided 2026-07-31):** the data
  partition is mounted without `dirsync`, so a write that must survive an
  immediate power cut uses the four-step fsync/rename pattern in
  `docs/runtime.md`. `dirsync` would tax every card write by every app —
  small synchronous writes are an erase-block rewrite on SD/eMMC — and still
  wouldn't remove the need to fsync file data. Don't relitigate without new
  evidence (e.g. third-party apps repeatedly getting it wrong); bean
  `gosd-0nk4` records the analysis and the one-word change site.
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
  bean's build even when the plan sequences them the other way. The reverse
  transition (internal → public) happens in ONE activation PR together with
  the `artifacts.Version` bump, only after the board's first artifacts
  release is published (pattern proven by gosd-7wv9; the pi-zero-w
  activation once went public before its tag existed and turned CI red).
  Pre-merge-test a new board's artifacts CI job by `workflow_dispatch`-ing
  `build-artifacts.yml` on the PR branch — the tag run must not be the job's
  first execution. Adding a
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
  procedure in `docs/artifacts.md`. Releases are cheap — cut an interim one
  rather than queueing bench-blocking fixes for a planned window (v0.7.0 and
  v0.8.0 shipped hours apart, 2026-07-26).
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
  above. That no-overlay constraint is **Rockchip-only**: Pi firmware
  applies overlays natively, and when a Pi feature needs one (e.g.
  `--usb-gadget` ships `overlays/dwc2.dtbo`, bean gosd-spjt) the `.dtbo` is
  pinned in the board manifest from the same raspberrypi/firmware commit as
  the GPU boot files — never assume the rule transfers between families.
- **Audit what a Pi defconfig hands you — three hardware-found traps in one
  week (2026-07):** bcmrpi/bcm2711 defconfigs ship `=m` drivers that the
  no-modules build promotes to `=y`, smuggling in unwanted built-ins
  (`mac80211_hwsim`'s phantom wlan0/wlan1 radios stole wifiup's interface
  pick — gosd-6nl2; the legacy gadget zoo claimed the only UDC as "Gadget
  Zero" before any configfs gadget could — gosd-spjt), and ship values that
  silently assume Pi-firmware cmdline injection
  (`SERIAL_8250_RUNTIME_UARTS=0` left the Zero W with no console at all —
  gosd-md4w). When adding a Pi board or touching its fragment, grep the
  recorded kernel.config for surprises and disable explicitly.
- **Know a Pi DTB's lineage before trusting driver bindings:** the pinned
  rpi tree builds both mainline-style DTBs (`bcm2835-*`) and downstream-style
  ones (`bcm2710-*`) with different compatibles and conventions. The
  downstream kernel's DMA path needs the downstream soc `dma-ranges`
  (gosd-1ey5 patches it into the Zero W's mainline-style DTB), and a usb
  node's compatible decides `dwc_otg` (downstream, host-only in practice)
  vs `dwc2` (mainline, gadget-capable) — opposite gadget outcomes
  (gosd-spjt). Check which driver a node's compatible binds at the pin.
- Kernel pins are **per-family**, bumped family-wide, never one board alone:
  the Rockchip boards + qemu-virt share one mainline stable tag
  (`internal/kernelspec`'s `fleetKernelTag`), while the Pi boards share one
  raspberrypi/linux **commit** pin (`piZeroCommitRef` — a DOWNSTREAM-tree
  pin, not mainline; assuming the Pis ran mainline sent gosd-anyp's research
  down a dead end for a day). Kernel/U-Boot Docker builds take 20-60 min:
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
- When a change touches `js/`: also run `cd js && pnpm install --frozen-lockfile
  && pnpm run format:check && pnpm run lint && pnpm run typecheck && pnpm run
  build && pnpm test && pnpm run test:integration` (the last needs Go, for the
  fixture generator).
- Run the gates — and any `gh pr checks` polling — in the FOREGROUND and read
  the results directly. Agents that parked on background monitors for a test
  run or CI watch stalled repeatedly (2026-08: six separate stalls, each
  leaving finished work unpushed until nudged). Poll CI with plain
  `sleep 60 && gh pr checks <n>` loops.
- Bizarre build failures while sibling agents/worktrees run on this machine —
  stdlib packages "not in std", ENOSPC, evicted cache entries — mean the
  shared Go build cache is contended or corrupted, not that your change is
  broken: re-run with an isolated `GOCACHE` (plus `golangci-lint cache
  clean`) before believing any of it.

## Code conventions

- Errors shown to CLI users must be actionable ("X failed because Y; try Z"),
  never bare wrapped chains.
- Tests are behavioral and concise; fixture-driven where the bean says so.
- Comments only where code can't explain itself; docstrings on exported API.
- Board or feature status changes must update COMPATIBILITY.md in the same PR.
- Anything that formats, adopts, or commits on-disk state needs an explicit
  crash-ordering argument (what is provably durable before the commit record
  lands) and an adversarial review pass BEFORE requesting JP's review. A
  filesystem probe is never proof a write completed — an interrupted format
  can leave probe-passing debris. The pattern that survives review is
  write → sync → marker → sync (dataexpand's `gosd-data-established`, the
  provisioning snapshot's digest-last `snapshot.json`); both exist because
  review caught probe-only gates adopting debris (gosd-lirl's rejection).
- FAT32 work goes through `internal/diskfmt`'s wrappers, never go-diskfs
  directly: go-diskfs under-sizes FATs at ~0.8% of volume sizes (mitigated in
  diskfmt; upstream patch recorded in gosd-e3e3), silently drops label spaces
  to per-field 8.3 trims (gosd-xq9l, gosd-f83b), and makes leading-dot
  filenames invisible to its own directory listings (documented on
  `diskfmt.CreateEmptyFile`).
- Raw netlink via `mdlayher/netlink` MUST OR `netlink.Request` into
  Execute/Send flags — the library does not add it, and the kernel silently
  SKIPS non-Request messages while still returning a success ack when
  `NLM_F_ACK` is set (a no-op dressed as success; with neither flag the call
  hangs forever). Two bench days went to this (gosd-anyp);
  `wifiup/connect_linux_test.go` pins the pattern — mirror it for any new
  raw-netlink call.
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
