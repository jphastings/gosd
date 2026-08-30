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
- PRs with user-facing changes need a change file (`.changeset/*.md`, format
  in docs/releasing.md — `internal/repocheck`'s change-file test validates
  every one in the tree, so `go test ./...` catches a malformed key), or the
  `no release notes` label. Releases happen by merging the knope release PR,
  not by tagging.
- JP reviews and merges every PR. Do **not** self-merge, even on green CI.
- CI must be green before requesting review. Include the bean file changes
  (status, checked todos, Summary of Changes) in the same PR as the code.
- Commit messages: imperative subject, body explains why. No test-result
  summaries in commit messages.
- Three `beans` CLI traps — the title being positional, the new id living at
  `.bean.id`, and `--body-replace-old/--body-replace-new` applying one pair per
  call — are enforced by a `PreToolUse` hook (`scripts/claude-guard.sh`,
  wired up in `.claude/settings.json`), as are self-merging and PRs to repos
  outside `jphastings`. The hook refuses the command and explains the reason
  and the fix at the moment it matters, so they are not restated here; read the
  script if you want the rules without triggering one.
- **An epic closes when its code has shipped, even if bench verification is
  outstanding** (2026-08-21): where an epic's implementation is on `main` and
  CI-proven and its only open child is hardware verification of that
  already-shipped code, close the epic with a `## Summary of Changes` naming
  the bench bean, and re-parent that bean to no parent so the tree never shows
  a completed epic with an open child. The boundary is the part that gets
  mistaken: this covers verification of work already delivered, NOT epics
  where the bring-up itself is the deliverable — `gosd-v370` (Radxa Zero 3E)
  stays open because bringing the board up IS the work.
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
  pi-zero-2w / pi-3b / pi-cm4 / radxa-zero-3e / nanopi-zero2 / rock-4se /
  cubie-a5e / qemu-virt / turing-rk1, and `GOARCH=arm GOARM=6` for pi-zero-w
  (BCM2835 is armv6, 32-bit only). The build pipeline compiles the app and
  gosd-init once per architecture needed by the selected boards (decided
  2026-07-06; was arm64-only).
- **Board IDs:** `pi-zero-2w`, `pi-zero-w` (epic gosd-ajpz),
  `pi-3b` (BCM2837, one image covers the 3B and 3B+ — epic gosd-xhc3),
  `pi-cm4` (Raspberry Pi Compute Module 4, Lite/no-wireless variant, BCM2711
  — the same defconfig pi-3b already builds against; SD-boot only in scope
  for now, eMMC deferred — epic gosd-7676, internal only until its
  artifacts release lands),
  `radxa-zero-3e`, `nanopi-zero2` (FriendlyElec RK3528A — epic gosd-cwjf),
  `rock-4se` (Radxa ROCK 4SE, RK3399-T — epic gosd-cuym),
  `cubie-a5e` (Radxa Cubie A5E, Allwinner A527 — first Allwinner board,
  epic gosd-h1wv), `turing-rk1` (Turing Machines RK1, RK3588 — the fleet's
  first board with no SD/microSD slot at all; boots from onboard eMMC only,
  flashed via USB maskrom/rkdeveloptool or the Turing Pi 2 BMC rather than a
  card reader, see `docs/turing-rk1-flashing.md` — epic gosd-bntd, internal
  only until its artifacts release lands); also `qemu-virt` (internal —
  see the "qemu-virt board" decision below: registered and buildable via
  explicit `--board=qemu-virt`, but excluded from `--help` text, the default
  build set, and catalog generation). `gosd build` with no `--board`
  builds **all** (public) boards, emitting `<appname>-<board>.img` next to
  each other; `--board` (repeatable) restricts.
- **Naming surfaces:** env vars `GOSD_*`; kernel cmdline params `gosd.*`;
  FAT partition labels, **per-app**: `<prefix>-boot` / `<prefix>-data`;
  boot-partition config tree at `config/` (one value per file — see
  `internal/configtree` and `docs/config.md`); app build tags — the bare `gosd`
  (set for every image gosd builds, so an app can gate device-only source
  with `//go:build gosd` and fall back with `//go:build !gosd`, bean
  `gosd-cm4b`) plus `gosd_<board-id>` (underscored, e.g.
  `gosd_pi_zero_2w`), both passed to the app compile only (see
  `boards.BuildTags` and `docs/board-build-tags.md`). **The label prefix**
  defaults to the sanitized app name truncated to 6 bytes, and is
  overridable via `gosd build --label-prefix` / `gosd run --label-prefix`
  (decided 2026-08-09, bean `gosd-lo7k`) — a clean break from the old fixed
  `GOSD-BOOT`/`GOSD-DATA` with no adoption alias: a card flashed by a
  pre-`gosd-lo7k` release fails the reflash-upgrade adoption gate on its
  data partition and is cleanly reformatted, never halted.
- **Default hostname:** the sanitized basename of the app's main package,
  overridable via a `config/hostname` file (`--boot-config-dir`, or hand-edited
  on the card).
- **Public API surface** (semver-relevant): `cmd/gosd`, `gadget/` (USB gadget
  library), `emmc/` (onboard-eMMC format/mount), `disk/` (the same for any
  attached mass storage — NVMe, USB drive, card reader), `sound/` (ALSA PCM
  playback: `Open`/`OpenWith`, a `Device` to `Play` frames to; needs a
  `gosd build-kernel` kernel, see `docs/sound.md`) and `fault/`
  (`Fatal(Report)` — records a user-actionable fatal error for gosd-init to
  write to `LAST_FATAL_ERROR.md` on the card and HALTS the device, never
  returns; plus `RegisterSecretString`, which writes through to
  `internal/secretreg`'s /run file on the call, not at crash time, so a
  panic the app never sees coming is still redacted — bean `gosd-aa1p`,
  epic `gosd-47z3`, see `docs/crash-reports.md`. Off a device — no `gosd`
  build tag — `Fatal` renders the identical Markdown to stderr and exits,
  which is a headline feature, not a fallback). Everything else lives
  under `internal/`; `emmc` and `disk` share `internal/blockmount` (the
  format/mount orchestration, label rules and candidate selection) and
  `internal/diskfmt` (pure-Go inspect/format — FAT32 via go-diskfs, exFAT
  written directly from the Microsoft spec, ext4 from a checked-in golden
  image), so a change to one must not silently change the other's
  semantics. Both `emmc` and `disk` take the same typed `Filesystem` token
  (`EXT4`/`FAT32`/`ExFAT`, one const set per package but identical in
  shape and spelling) whose zero value is `EXT4` (epic `gosd-lfu0` for
  `disk`, bean `gosd-9sc4` for `emmc` — see the dedicated locked decisions
  below for the format/grow/adopt mechanism, a one-liner is
  `internal/blockmount`'s package doc). exFAT and ext4 each need their
  kernel support (`CONFIG_EXFAT_FS`/`CONFIG_EXT4_FS`) checked against
  `/proc/filesystems` before any write. The two packages still differ in
  candidate *selection*, not filesystem: `emmc` addresses exactly one
  device (the board's onboard eMMC) with no equivalent of `disk`'s
  multi-class ranking or `FormatAndMountDevice`/`Devices`.
- **`disk/` defaults to ext4 (decided 2026-08-07, epic `gosd-lfu0`):**
  `disk.Options.Filesystem`'s zero value is `disk.EXT4` — a deliberate
  breaking change from the prior FAT32 default, shipped as a minor version
  bump with a release-notes-level callout (bean `gosd-ucgr`). Formatting
  writes a checked-in golden ext4 image (`internal/diskfmt/ext4golden`)
  straight to the device, then grows it online to the disk's actual size
  exactly once, at first establishment (`EXT4_IOC_RESIZE_FS`,
  `internal/blockmount`) — no `mkfs`/`resize2fs` ever runs on-device. A
  later mount of an already-established volume adopts it, gated on a
  hidden completion marker (the same write → sync → marker → sync
  discipline as every other on-disk commit in this codebase — see
  `internal/blockmount`'s package doc for the full crash-ordering
  argument), never a probe. The journal buys metadata crash-consistency
  and mount-time replay, not data durability — the four-step fsync
  pattern (docs/runtime.md) remains the app-facing contract regardless of
  filesystem. FAT32/exFAT remain available as explicit `Options.Filesystem`
  tokens for removable media meant to be read on another host, which is
  the case ext4's default does not serve. Proven end-to-end (format, grow,
  a hard qemu kill with no clean shutdown, reboot, adopt, journal replay)
  by CI's `qemu-disk-ext4` job; real-hardware verification is bean
  `gosd-vv5o`.
- **`emmc/` shares `disk`'s Filesystem token and ext4 default (decided
  2026-08-07, bean `gosd-9sc4`) — this REPLACES the prior "emmc is
  FAT32-only by design" locked decision.** `emmc.Options.Filesystem`'s
  zero value is `emmc.EXT4`, mirroring `disk.Options.Filesystem` token for
  token (`emmc.EXT4`/`emmc.FAT32`/`emmc.ExFAT`) — the same deliberate
  breaking default, shipped in the same CLI minor release as `disk`'s flip
  (bean `gosd-2194`). The format/grow/adopt machinery was already shared
  via `internal/blockmount`'s `runEXT4`; this bean only stopped `emmc`
  pinning `diskfmt.FAT32` and let its own token flow through, so an
  established FAT32 eMMC volume plus a zero-value (ext4) request refuses
  without `Destructive: true`, naming the upgrade story in the error (pass
  `emmc.Options{Filesystem: emmc.FAT32}` explicitly to keep adopting the
  existing FAT32 volume; `Destructive: true` reformats it as ext4 and
  loses its data). What does **not** change: `emmc`'s candidate selection
  stays a single onboard device (`chooseEMMC`, keyed on `Kind == "MMC"`)
  with no `disk`-style multi-class ranking or
  `FormatAndMountDevice`/`Devices` equivalent, and its hardware-partition
  exclusion still relies on the `Kind == ""` sysfs quirk rather than
  `disk.rank`'s explicit regex (`gosd-ix38`) — that split predates this
  bean and is unrelated to which filesystem is requested; see
  `internal/blockmount`'s package doc for the full detail.
- **GOSD-DATA is FAT32 by default; ext4 is an opt-in (decided 2026-08-09,
  bean `gosd-95yu`) — this AMENDS `gosd-lfu0`'s "/data on the SD card stays
  FAT" non-goal without overturning it.** `gosd build --data-filesystem`
  takes `fat32` (default) or `ext4`. FAT32 stays the default because the
  point of the default is that a flashed card reads in any computer's SD
  reader; ext4 is for apps that need `/data` to survive rapid power-off, and
  pays for it by being unreadable — and unrepairable — from a macOS or
  Windows host. The journal buys metadata crash-consistency and mount-time
  replay, never data durability: `docs/runtime.md`'s fsync sequence is the
  app-facing contract for both filesystems. The choice is baked into
  config.json only (no config-tree key, no `GOSD_*` override) and is part of
  the app's on-card ABI, like `--boot-size`. Refused at build time for any
  selected board whose pinned kernel lacks `CONFIG_EXT4_FS` — which matters
  because a bare `gosd build` builds every public board; see
  `boards.EXT4Support`. **No board GoSD currently ships is refused** — bean
  `gosd-ssth` corrected an earlier, snapshot-derived belief that the Pi
  family had no ext4 — but the mechanism stays for any future board whose
  stock kernel doesn't build the option in. `--data-flush` is refused
  alongside it
  (`flush` is a vfat-only mount option), and `dataexpand`'s 256GiB
  `maxPartitionBytes` is FAT32-only, so ext4 `expand` fills the whole card.
  **Consequence worth knowing before touching either package:** because
  `diskfmt.FormatEXT4` writes a fixed 512MiB golden and cannot grow in Go,
  EVERY ext4 data partition — fixed-size as well as `expand` — ships that
  golden and grows once on first boot via `EXT4_IOC_RESIZE_FS`, so
  `dataexpand` now runs for any ext4 image rather than only `expand` ones.
  The two paths carry deliberately different crash-safety arguments: the
  expand path keeps "an MBR entry only ever exists over a filesystem proven
  finished", while the fixed-size path (entry already present) only ever
  GROWS AND MARKS, never formats — growing is non-destructive and the resize
  ioctl no-ops once the filesystem already fits, which is why it needs none
  of `blockmount.runEXT4`'s `RootHasOtherContent` second opinion.
- **The ext4 golden's "8 TiB" is not a ceiling — don't encode it as one.**
  `internal/diskfmt/ext4golden/manifest.json`'s `verifiedGrowthCeilingBytes`
  reads like a capability limit and is not: 8 TiB is (a) the cap of
  `resize_inode`, the approach the recipe deliberately REJECTED — its
  reserved GDT blocks are reached through a single indirect block, a hard
  `blocksize/4` cap — and (b) a build-host artifact, because `build.sh` tries
  16 TiB and falls back until the host's own filesystem can represent a file
  that large (colima's VM root is a non-64bit ext4). The golden ships
  `^resize_inode,meta_bg` precisely to escape that, and `meta_bg` is
  documented to 2^32 groups / 512 PiB. Capping anything at 8 TiB would
  re-impose the limitation `meta_bg` was chosen to remove. Re-proving the
  real ceiling is bean `gosd-2ssb`. Relatedly, the golden is 512MiB because
  the 128MiB journal can NEVER be resized after format and so must be sized
  for the grown volume's whole life — a floor on the seed, not a cap on the
  result (`ext4golden/README.md` has the full argument).
- **Layout ABI (decided 2026-07-31, docs/design/upgrade-path.md):** the boot
  volume size is per-app (`gosd build --boot-size`, default 256MiB) and is
  that app's on-card ABI — changing it in a later release erases the data
  partition on upgrade (cleanly, via the adoption gate) and is a
  release-notes-level breaking change. **The data-partition filesystem
  (`--data-filesystem`) and the boot/data label pair (`--label-prefix`,
  see the "Naming surfaces" decision above) are on-card ABI the same way**
  (bean `gosd-95yu`, bean `gosd-lo7k`, both 2026-08-09): changing either
  between releases also fails the adoption gate and cleanly reformats the
  data partition on the next reflash-upgrade, never a halt, and the boot
  partition is unaffected. Nothing on-device may assume a fixed
  data-partition offset: derive it from the flashed MBR (partition 1 start
  + size), the way dataexpand does. Plain Imager reflash is the baseline
  upgrade path: `--data-size=expand` images keep the data partition via
  first-boot re-adoption (now gated on this image's configured data label,
  matched case-insensitively), and the config store in /data
  (`cmd/gosd-init/internal/configstore`, bean gosd-87ip) puts the settings
  somebody put on the card back onto the newly flashed one.
- **vfat `flush` is opt-in, default off (decided 2026-08-02, bean
  gosd-9m1k):** normal writeback everywhere (`gosd build --data-flush` /
  the card's `config/data_flush` / env `GOSD_DATA_FLUSH` to opt in). Durability
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
  compliance). CLI releases are cut by merging the knope release PR, which
  produces a `gosd/vX.Y.Z` GitHub release plus the plain `vX.Y.Z` Go-module
  tag, and pin an artifact version.
  Developers never *have to* compile a kernel themselves — `gosd build-kernel`
  (epic gosd-47rm) is an opt-in path for compiling in a driver GoSD's stock,
  trimmed kernels cut; see `docs/custom-kernels.md`.
- **Kernels are monolithic forever: no loadable modules (decided 2026-08-20,
  bean gosd-2k9p).** `CONFIG_MODULES=n` fleet-wide, and the question is closed
  rather than deferred — don't design around a future `.ko` loader, and don't
  read the reserved `[[module]]` table in `gosd-kernel.toml` as a planned
  feature. The only capability modules add over `gosd build-kernel`'s
  compile-in path is loading a driver without a rebuild, and those rebuilds are
  content-addressed, cached and container-local. Saying yes would cost,
  permanently: a fleet-wide kernel bump; a kernel-devel artifact (headers,
  `.config`, `Module.symvers`, release string) per board per arch every
  artifacts release, because vermagic and MODVERSIONS CRCs mean a `.ko` must be
  built against GoSD's *exact* kernel and no distro's module can ever be
  dropped in; and a module-signing decision that widens the trust boundary of
  an appliance that deliberately has no interactive surface at all. It reopens
  only for a case compiled-in drivers genuinely can't serve — proprietary
  out-of-tree drivers, or ecosystems where *end users* rather than the
  developer attach hardware the image was never compiled for.
- **On-disk caches must stay bounded to the current working set (decided
  2026-08-08, bean gosd-gdro):** nothing gosd caches on a user's machine may
  grow in proportion to how many times or how many *versions* of gosd are run.
  The download caches under `os.UserCacheDir()/gosd/` (`artifacts/<version>`,
  `cacerts`, `ingress`, `kernel-firmware`) auto-prune to the current pins after
  a successful build — keep the current version's assets (~hundreds of MB is
  fine), drop superseded ones. The durable `build-kernel`/`build-external`
  state dir (`defaultBuildRoot`, deliberately NOT under `UserCacheDir` per
  gosd-l4y9) is bounded separately by keep-last-N because its entries cost
  20-75 min to rebuild (follow-up gosd-9o73). Don't reintroduce unbounded
  per-version accumulation, and don't make pruning fail a build (best-effort,
  only gosd's own cache dirs, never on `--artifacts-dir` runs).
- **CA roots ship in every image (decided 2026-08-07, bean gosd-kzgq):** the
  pinned Mozilla bundle lands at `/etc/ssl/certs/ca-certificates.crt` in every
  initramfs, so app HTTPS needs no `x509roots/fallback` import. The pin is
  `internal/cacerts` — a DATED curl.se snapshot URL + sha256, never the rolling
  `cacert.pem` URL (it changes under a pin); bump procedure in the package doc.
  Don't remove the bundle to save space, and don't document the blank-import
  as required (it remains a valid app-side alternative only).
- **End-user flashing path (decided 2026-07-05):** the flagship flow is a
  Raspberry Pi Imager custom-repository catalog entry — `gosd build` can emit
  an `os_list.json` entry declaring `init_format: "cloudinit"`, the developer
  hosts it next to their image, and end users paste the repo URL into Imager's
  Settings → Custom repository to get the full WiFi/hostname wizard.
  Hand-editing the config tree is the always-present fallback (works with any
  flasher). Consequence: gosd-init's provisioning parser reads cloud-init
  YAML + the config tree; `firstrun.sh` parsing is out of scope (log-and-point
  -at-the-config-tree if encountered). See docs/provisioning-formats.md.
- **qemu-virt board:** an internal-only board profile for CI and local
  testing (`qemu-system-aarch64 -M virt`, virtio, SD appears as /dev/vda).
  It is EXCLUDED from default all-boards builds and from end-user docs;
  build it only via an explicit `--board=qemu-virt`.
- **gosd-init has no interactive surface**: no shell, no SSH, no remote debug,
  ever. Serial console output and app logs only. **mDNS is the only network
  listener in gosd-init** — full stop, with no sanctioned exception pending.
  The one that used to be pending, the OTA update endpoint, is gone: OTA was
  dropped entirely on 2026-08-21 (JP) and reflashing is the permanent, only
  update path (epic gosd-vxal and its whole chain, scrapped; the design record
  is kept in docs/design/ab-updates.md, marked decided-against).
  cloudflared (when baked via `--ingress cloudflared`) is an outbound-only
  tunnel supervised by gosd-init — still no listeners, no shell. Same for
  tailscale-funnel (`--ingress tailscale-funnel`, epic gosd-65uy): the shim
  runs entirely over tsnet's userspace netstack, dialing out over WireGuard
  rather than binding a socket on any real host interface, so Funnel makes
  the app publicly reachable without gosd-init or the shim adding a listener
  either.
- **A gosd-shipped subprocess must not assume a normal OS environment
  (bench-proven 2026-08-08, bean gosd-6cf2).** gosd-init launches supervised
  children with a minimal, explicit env (e.g. tsfunnel gets only
  `TS_AUTHKEY`) on an initramfs rootfs with no `HOME`, no `/var/lib`, no
  `os.UserCacheDir()`. Third-party libraries that quietly need those PANIC or
  wedge only on-device — fakes can't catch it. Two concrete lessons that
  generalise: (1) set whatever dir env a library needs EXPLICITLY (tsnet's
  `TS_LOGS_DIR` → the state dir; cloudflared's `HOME` → `/run/gosd/...`),
  don't rely on OS defaults; (2) a library that treats a
  present-but-unparseable state file as fatal and never regenerates it wedges
  the device — tsnet does this even though it writes those files ATOMICALLY
  (write→fsync→rename, since 2022), so the corruption comes from the FAT32/SD
  media on power loss, NOT a torn write or a missing write→rename — and the
  wedge is STICKY because `/data` survives reflash and can't be cleared from a
  macOS host (ext4), so the shim must self-heal a corrupt/empty state file
  (drop-if-unparseable) rather than trust it (that drop is only a partial
  mitigation: valid-JSON-but-wrong-schema still wedges — bean gosd-6cf2). Also: never
  `defer thing.Close()` unconditionally on a start-failure path — tsnet's
  Close panics when Up failed early and MASKS the real error; only defer
  Close after a successful start. **Corollary for bench triage:** an
  on-device failure that reproduces off-device (macOS/qemu) is a code bug;
  one that does NOT (e.g. gosd-h46e's tsnet-404, present on linux/arm64 board
  but not macOS with the same binary+key+network) is environment-specific —
  reproduce it under qemu-virt / a linux-arm64 container before burning
  reflash cycles, and verify network claims with an on-device preflight
  (`http.Get` to the endpoint) before blaming the network.
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
- **The nix flake bundles Go twice, and go.mod's floor is not ours to choose
  (decided 2026-08-09, bean gosd-jm2v):** `flake.nix` needs Go both to compile
  gosd (`buildGoModule` sets `GOTOOLCHAIN=local` and a nix sandbox has no user
  PATH — there is no toolchain fetch to fall back on, so `pkgs.go` must already
  satisfy go.mod) and at run time, where the wrapper appends it to PATH with
  `--suffix` so a user's own toolchain still wins. Keep `--suffix`: gosd
  compiles the *user's* app, so someone who installed a newer Go should get it;
  the bundle exists so `nix run github:jphastings/gosd -- build ./cmd/myapp`
  works on a machine with no Go, as README promises. **go.mod's `go` directive
  is dependency-derived** — `go mod tidy` raises it to the maximum of every
  dependency's own floor (`tailscale.com` sets today's `1.26.5`), so it cannot
  be hand-relaxed to a bare major.minor and a `go mod tidy` would undo the
  attempt. When a dependency bump raises it, **`nix flake update` belongs in
  the same PR**; if nixos-unstable hasn't shipped that Go patch release yet the
  `nix build` job stays red until it does — a wait, not a broken change (it
  cost PR #231 once). gosd never compares Go versions itself: `GOTOOLCHAIN=auto`
  is Go's default and transparently fetches a newer toolchain, so an up-front
  check would reject setups that work. `internal/build.CheckToolchain` only
  asserts a `go` exists; `explainBuildFailure` recognises Go's own floor error
  and appends remediation, naming no version of its own (the floor that tripped
  may be the user's app's, not ours).
- **A per-attribute config tree replaces the old single hand-editable boot
  file wholesale (decided 2026-08-12/13, epic `gosd-rw6n`).** One value per
  file, newline-trimmed, empty means unset; padding is the reservation, a
  value ships padded to at least 256 bytes and one shipped larger reserves
  its own size; every value needs an `.explain.md` sidecar (its own or
  inherited), required at build, never required at runtime. gosd ships its
  own defaults tree, overlaid file-by-file by an app's own
  `gosd build --boot-config-dir`. The injection manifest carries a `config`
  array alongside `placeholders`, and the TypeScript client gained a
  matching `config` option. A `/data` store (`cmd/gosd-init/internal/configstore`,
  bean `gosd-87ip`) keeps whatever differs from the shipped tree and
  restores it only when a *different* image identity boots, surfacing a
  new image's own changed default as `<name>.new` and an orphaned setting
  as `<name>.unused` — presence in the store is the whole record of
  intent, so putting a value back exactly as shipped (or reflashing the
  same image) is indistinguishable from a revert and stops it being kept.
  A Raspberry Pi Imager cloud-init seed is consumed — read, then durably
  deleted, then written into the tree — rather than kept as a competing
  source, so a wizard's answers become ordinary settings from that point
  on. `--env`, `--env-file`, `--hostname`, `--wifi-ssid` and `--wifi-pass`
  are removed: the overlay directory is the developer's input, the wizard
  (or a hand-edit) is the operator's. See `docs/config.md`.

## Board work & artifact releases

- **Bench/bring-up knowledge that isn't a locked decision lives in
  `docs/development/<board-id>.md`, one file per board** — real hardware
  surprises, debugging techniques that worked, DT-research assumptions
  later corrected on real hardware, benign-vs-real warning signs. This
  file stays for locked decisions (the board's own permanent facts);
  `docs/development/` is for the "how we found that out" a future
  bring-up would otherwise have to rediscover by hand. Boards hosted on a
  Turing Pi 2 also share
  [the shared Turing Pi 2 BMC notes](docs/development/turing-pi-2-bmc.md)
  for BMC/tooling knowledge that isn't specific to any one node type.
  Update the relevant file during or right after a bench session, the
  same way a locked decision that proves wrong gets corrected here.
- **Kernel-build source of truth is `internal/kernelspec`** (a declarative
  Go `KernelSpec` per board), not shell scripts — `gosd build-kernel`
  (`internal/kernelbuild`) reads it directly. Change a board's kernel build
  there, not by hand-editing a retired `build.sh`/`docker-build.sh`.
- **`build/boards/*/kernel.config` is a stale snapshot, never the source of
  truth about what a kernel can do.** It is only rewritten by an actual
  `gosd build-kernel` run, so it lags the board's `kernel.fragment` — which
  is the assertion — by however many releases. Check a capability claim
  against the fragment, or better against the published artifact
  (`gh release download artifacts/vX.Y.Z -p '<board>.tar.zst'` carries the
  real `kernel.config`, and `strings` on the kernel proves the driver was
  compiled in; gunzip a 32-bit zImage's payload first or `strings` reads as
  a false negative). Bean `gosd-95yu` read "the Pi family has no ext4" off
  these snapshots months after the fragments gained it, and that one wrong
  fact reached a build-time refusal, a user-facing runtime error and a
  published release note before `gosd-ssth` undid it.
- **Building a board's kernel needs the board *registered*, not just a
  `kernelspec` entry.** `gosd build-kernel --board <id>` resolves `<id>`
  through `internal/boards` (registered in `internal/boardset`) *before*
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
  first execution. `internal/kernelspec/kernelspec_test.go` enumerates the
  fleet from `internal/boardset`, so a new board reaches its board-count and
  outputs-vs-`Artifacts()` checks on registration alone; the one list there
  still hand-maintained is the Rockchip DTS-patch allowlist (a board either
  should or should not carry patches — only a human knows which).
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
  turns the qemu boot-to-HTTP CI job red. It lands with an `artifacts:`
  change file (see `docs/releasing.md`); merging the knope release PR once
  it lists `artifacts` creates the tag and GitHub release, and a separate
  follow-up PR then bumps `Version` and verifies against the real release.
  Full procedure in `docs/artifacts.md`. Releases are cheap — cut an interim one
  rather than queueing bench-blocking fixes for a planned window (v0.7.0 and
  v0.8.0 shipped hours apart, 2026-07-26).
- **`internal/artifacts.Version` never moves alone.** `ManifestSHA256` beside
  it pins that release's `manifest.json`, which is the trust anchor every
  per-file digest is read through (bean gosd-1jjh): the manifest's bytes are
  re-hashed against it on download AND on every cache read, so a self-
  consistent poisoned pair in `~/.cache/gosd/` can no longer backdoor every
  image built afterwards. `build/artifacts/pin-bump.sh` writes both; a
  hand-edit that moves one fails every build (deliberately — it fails closed).
  `--artifacts-dir` is checked before any of this and is untouched by it.
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
- **Audit what a Pi defconfig hands you.** bcmrpi/bcm2711 defconfigs ship
  `=m` drivers that the no-modules build promotes to `=y`, smuggling in
  unwanted built-ins, and ship values that silently assume Pi-firmware
  cmdline injection — both classes have bitten real boards in this fleet
  (see a board's own `docs/development/<board-id>.md` for the specifics).
  When adding a Pi board or touching its fragment, grep for surprises and
  disable explicitly — but grep the *released* kernel.config, not the
  committed snapshot (see above), and confirm against the running board
  before "fixing" one: firmware cmdline injection can make the identical
  defconfig default harmless on one board and fatal on another.
  `/proc/cmdline` on the booted board settles it. Also cut the legacy USB
  gadget zoo (`drivers/usb/gadget/legacy/`) explicitly on any board getting
  `--usb-gadget` support — a built-in gadget driver there claims the UDC
  before gosd's own configfs gadget can (bean gosd-spjt).
- **Know a Pi DTB's lineage before trusting driver bindings:** the pinned
  rpi tree builds both mainline-style DTBs (`bcm2835-*`) and downstream-style
  ones (`bcm2710-*`) with different compatibles and conventions — a USB
  node's compatible alone decides whether a controller binds `dwc_otg`
  (downstream, host-only in practice) or `dwc2` (mainline, gadget-capable),
  and a mainline-style DTB's DMA needs an explicit peripheral-window
  `dma-ranges` entry the downstream one already carries. Check which driver
  a node's compatible binds at the pin before assuming a capability; see a
  board's own `docs/development/<board-id>.md` for specific instances this
  caught.
- Kernel pins are **per-family**, bumped family-wide, never one board alone:
  the mainline-fleet boards — Rockchip, Allwinner (cubie-a5e), and
  qemu-virt — share one mainline stable tag (`internal/kernelspec`'s
  `fleetKernelTag`), while the Pi boards share one raspberrypi/linux
  **commit** pin (`piZeroCommitRef` — a DOWNSTREAM-tree pin, not mainline;
  assuming the Pis ran mainline sent gosd-anyp's research down a dead end
  for a day). Kernel/U-Boot Docker builds take 20-60 min: run them
  backgrounded and poll the log, never in a foreground shell — and from the
  session that owns them, never from a subagent's background task (a
  subagent's background jobs are killed when it returns; the cubie-a5e
  U-Boot build died this way, log frozen mid-compile with no error).
- **A new Allwinner board needs its own U-Boot defconfig merged upstream
  first** — per-board LPDDR4 DRAM tuning lives there, so there's no generic
  Allwinner DRAM init to fall back to. U-Boot defconfigs can be RENAMED
  between the mailing-list posting and the merge — verify against the tree
  at the pinned tag, never the posting name (see
  [this board's development notes](docs/development/cubie-a5e.md) for how
  this played out on the fleet's first Allwinner board, epic gosd-h1wv).
- **Adding or activating a board is checked mechanically**, so don't work
  from a remembered list: `internal/repocheck/boards_test.go` derives from
  the board registry that every board has a `kernelspec` entry and a
  `build/boards/<id>/` directory, and that every *public* board's artifacts
  have fixtures in `cmd/gosd/testdata/fake-artifacts/` and a bring-up row in
  COMPATIBILITY.md — each in both directions, with the failure naming the
  edit to make. Flipping `registerInternal` → `register` starts demanding the
  last two by itself. The fixtures are checked because a missing one falls
  through to a real release fetch that a warm artifact cache in the real HOME
  satisfies locally, so the network tripwire only ever fires in CI (PR #205).

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
- When a change touches `js/`, **or `internal/inject`, `internal/configtree`
  or `internal/image`** (their build-time injection manifest / config-tree
  padding / byte-range logic is mirrored independently by the TypeScript
  downloader, `js/packages/gosd`, and only `test:integration` below exercises
  both sides against each other — bean gosd-uh94): also run
  `cd js && pnpm install --frozen-lockfile && pnpm run format:check && pnpm
  run lint && pnpm run typecheck && pnpm run build && pnpm test && pnpm run
  test:integration` (the last needs Go, for the fixture generator). This is a
  local-workflow shortcut, not the only backstop: `ci.yml`'s `js` job has no
  path filter and is a required status check, so it already runs (and can
  already block merge) on every PR regardless of what changed — running it
  locally first just catches a break before pushing instead of after.
- Run the gates — and any `gh pr checks` polling — in the FOREGROUND and read
  the results directly. Agents that parked on background monitors for a test
  run or CI watch stalled repeatedly (2026-08: six separate stalls, each
  leaving finished work unpushed until nudged). **Poll CI with
  `gh pr checks <n> --watch --interval 30`**, which blocks in the foreground,
  prints as it goes, and exits when the checks settle. Do NOT use a
  `sleep`-based loop: the agent harness hard-blocks foreground `sleep`, so
  this instruction previously guaranteed the very failure it exists to
  prevent — six of seven agents on one epic hit the block, improvised a
  background monitor, and stalled mid-task.
- Bizarre build failures while sibling agents/worktrees run on this machine —
  stdlib packages "not in std", ENOSPC, evicted cache entries — mean the
  shared Go build cache is contended or corrupted, not that your change is
  broken: re-run with an isolated `GOCACHE` (plus `golangci-lint cache
  clean`) before believing any of it.

## Code conventions

- Markdown docs hyperlink a **descriptive phrase**, never a file path — not in
  prose and not as a link's text: `[the runtime contract](runtime.md)`, not
  `` [`docs/runtime.md`](runtime.md) ``. COMPATIBILITY.md is the exemplar.
  `internal/repocheck` ratchets this rather than enforcing it — per-file counts
  seeded from the tree, which may not rise — so the existing violations stay
  put and only new ones fail (bean gosd-nw6e). Fixing prose and lowering a
  count is welcome, never required. This file is exempt: it is an index for
  agents, where the bare path is the useful form.
- Errors shown to CLI users must be actionable ("X failed because Y; try Z"),
  never bare wrapped chains.
- Tests are behavioral and concise; fixture-driven where the bean says so.
- Comments only where code can't explain itself; docstrings on exported API.
- Board or feature status changes must update COMPATIBILITY.md in the same PR.
  Its bring-up table is enforced (`internal/repocheck` asserts one row per
  public board, keyed on `DisplayName()`); the board × feature table below it
  is not — its column headers are deliberately abbreviated — so a feature's
  row is still yours to remember.
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
  `diskfmt.CreateEmptyFile`). Exactly three non-test packages may import
  go-diskfs: `internal/diskfmt` (the wrappers themselves), `internal/image`
  (the image assembler) and `internal/qemurun` (opens a built image
  read-only); tests may too, since reading a built `.img` back is the
  verification idiom this file mandates elsewhere. That fence is a depguard
  rule in `.golangci.yml`, so a fourth importer is a lint failure rather than
  something review has to catch — and `.golangci.yml` exists for that rule
  alone (`linters.default: standard` is exactly the set CI ran when there was
  no config at all).
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
  **CI enforces that with a `./examples/...` wildcard** in ci.yml's
  `smoke-build` job, one leg per arch — so a new example is covered the moment
  its directory exists, and there is no exemption mechanism: an example that
  can't build for a board arch is a bug in the example. Never re-expand
  either leg into per-example packages; the hand-maintained lists that
  preceded the wildcard drifted (bean `gosd-asdg`: `sattrack` was in neither
  leg, `usbserial` missing from armv6), and
  `internal/repocheck/examplesci_test.go` now fails if anyone reintroduces
  them. The armv6 leg also names `./sound` outright, because its ALSA ioctl
  request numbers embed struct sizes that differ 32-vs-64-bit and armv6 is
  the only place those compile-time assertions get checked.
  `examples/sattrack` is the reference for a bigger example: third-party deps
  when its bean justifies them, an in-tree `gosd build-kernel` recipe
  (`examples/sattrack/kernel/`), and graceful no-display degradation.
