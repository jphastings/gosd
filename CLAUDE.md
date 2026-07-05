# GoSD — guide for implementing agents

GoSD turns a Go main package into flashable SD-card images for small ARM boards
(Raspberry Pi Zero 2W, Radxa Zero 3E). See README.md for the pitch. All work is
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

## Project-wide locked decisions

- **Module path:** `github.com/jphastings/gosd`. **License:** MIT (LICENSE file).
- **Language:** pure Go everywhere; `CGO_ENABLED=0`. No build step may require
  root, Docker, or Linux — `go test ./...` must pass on macOS and Linux.
  Linux-only runtime code goes behind build tags.
- **Target:** `GOOS=linux GOARCH=arm64` only (both boards are arm64).
- **Board IDs:** `pi-zero-2w`, `radxa-zero-3e`. `gosd build` with no `--board`
  builds **all** boards, emitting `<appname>-<board>.img` next to each other;
  `--board` (repeatable) restricts. Reserved for planned support:
  `nanopi-zero2` (FriendlyElec, RK3528A — epic gosd-cwjf; gated on verifying
  mainline kernel/U-Boot support, since vendor images run a BSP kernel and
  GoSD is mainline-only; WiFi is an optional M.2 module there, Ethernet-first).
- **Naming surfaces:** env vars `GOSD_*`; kernel cmdline params `gosd.*`;
  FAT partition labels `GOSD-BOOT` / `GOSD-DATA`; boot-partition config file
  `gosd.toml`.
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
- **gosd-init has no interactive surface**: no shell, no SSH, no remote debug,
  ever. Serial console output and app logs only. The only network listeners in
  gosd-init are mDNS (and, later, the explicitly-designed update endpoint).
- **WiFi scope:** WPA2-PSK and open networks only through v0.x. WPA3/EAP are
  out of scope — log clearly when encountered.
- **Supported CLI hosts:** macOS and Linux (amd64/arm64), enforced by CI.
  Windows is untested best-effort; don't break it gratuitously.

## Quality gates — run ALL of these before every commit/PR

- `go test ./...`
- `go vet ./...`
- `gofmt -l .` (must print nothing)
- `golangci-lint run ./...` AND `GOOS=linux golangci-lint run ./...` — CI lints
  from Linux, so the second invocation is the one that must match CI; run both
  so darwin-only and linux-only files are each checked. A finding that is a
  cross-GOOS false positive (symbol only used in a `_linux.go` file) gets a
  `//nolint:<linter> // <reason>` comment, not an exclusion rule.

## Code conventions

- Errors shown to CLI users must be actionable ("X failed because Y; try Z"),
  never bare wrapped chains.
- Tests are behavioral and concise; fixture-driven where the bean says so.
- Comments only where code can't explain itself; docstrings on exported API.
