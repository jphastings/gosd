---
# gosd-zh95
title: 'Cubie A5E: artifacts release + board activation'
status: in-progress
type: task
priority: normal
created_at: 2026-08-06T22:34:12Z
updated_at: 2026-08-07T16:37:36Z
parent: gosd-h1wv
blocked_by:
    - gosd-o7jv
    - gosd-n6w9
    - gosd-axtv
---

Wire cubie-a5e into .github/workflows/build-artifacts.yml (kernel + uboot jobs mirroring rock-4se's, uploading u-boot-sunxi-with-spl.bin), pre-merge-test the new jobs via workflow_dispatch on the PR branch (the tag run must not be the jobs' first execution), then — tag-first, bump-second — after JP pushes artifacts/vX.Y.Z: a follow-up PR bumps internal/artifacts.Version, flips the board public (Register instead of RegisterInternal) in the SAME activation PR, and updates COMPATIBILITY.md + docs.

## Todos

- [x] build-artifacts.yml: cubie-a5e-kernel + cubie-a5e-uboot jobs + release-assembly wiring (single u-boot-sunxi-with-spl.bin, not idbloader/itb pair)
- [x] internal/boards artifacts mapping + tests for the new tarball contents
- [x] workflow_dispatch pre-merge run green on the PR branch (run 31178515476, success)
- [x] PR 1 (#191) merged; JP pushed artifacts/v0.9.0 (2026-08-07; a stray plain v0.9.0 tag was pushed first and deleted — the artifacts/ prefix is load-bearing for the workflow trigger)
- [x] Verify the artifact bump three ways (clean-HOME build, offline cache re-run, content spot-check e.g. dtc on the released DTB) — record in this bean
- [ ] PR 2: artifacts.Version bump + public registration + COMPATIBILITY.md/docs/catalog in one activation PR

## Progress

This bean splits into two PRs per the tag-first/bump-second rule (CLAUDE.md's
"Board work & artifact releases"). PR 1 (this landing) is CI wiring only:

- Added cubie-a5e-kernel and cubie-a5e-uboot jobs to build-artifacts.yml,
  mirroring rock-4se's pair, with the one structural difference the bean
  called out: the uboot job uploads a SINGLE file
  (build/boards/cubie-a5e/uboot/out/u-boot-sunxi-with-spl.bin), not an
  idbloader/itb pair, since the sunxi BootROM loads one SPL+FIT image where
  Rockchip needs two.
- Wired both into package-and-release's needs/download-artifact steps,
  extended the U-Boot source-provenance jq step to fold in cubie-a5e's
  from-source TF-A pin (mirroring rock-4se's blob-free pattern, reading
  build/boards/cubie-a5e/manifest.json's tfa section), and added
  dist/cubie-a5e.tar.zst to the release file list (same treatment as the
  already-internal qemu-virt tarball — internal-only boards still get
  packaged into every release, they're just excluded from --help/docs/the
  default build set).
- Updated docs/artifacts.md's board lists, staging-layout example, and
  release-file listing to match.
- internal/boards artifacts mapping (item 2) turned out to be ALREADY
  COMPLETE: internal/boards/cubiea5e/board.go's Artifacts()/board_test.go and
  internal/kernelspec/kernelspec_test.go's board-enumerating lists (allBoardIDs,
  the KernelSpec-outputs-vs-Artifacts map, the DTS-patch allowlist) already
  cover cubie-a5e in full — landed by this bean's blocking prerequisites
  (gosd-o7jv/gosd-n6w9/gosd-axtv). No new Go code was needed there; verified
  by running the full test suite.
- No internal/artifacts.Version bump, no Register() flip, no COMPATIBILITY.md
  change in this PR — those are PR 2's activation work, gated on JP pushing
  the artifacts/vX.Y.Z tag.
- Quality gates (go test ./..., go vet ./..., gofmt -l ., golangci-lint run
  ./... both darwin and GOOS=linux, actionlint) all green.
- Triggered the required pre-merge workflow_dispatch run on the PR branch
  per CLAUDE.md's "the tag run must not be the jobs' first execution" rule:
  https://github.com/jphastings/gosd/actions/runs/31178515476 (PR #191). Not
  watched to completion here — the orchestrator monitors it. Remaining todos
  (green run, PR merge, tag push, three-way verification, PR 2 activation)
  are all downstream of that run and of JP's tag push, so status stays
  in-progress.

## Three-way verification of artifacts/v0.9.0 (2026-08-07)

1. Clean-machine build: fresh HOME, no --board/--artifacts-dir → all SEVEN public images built (cubie-a5e now among them) from a real release download; hello-cubie-a5e.img = 285,212,672 bytes. (First attempt failed on host disk exhaustion — 1.4GiB free; reclaimed ~10GB of session build debris and re-ran clean.)
2. Offline re-run: same HOME, HTTP(S)_PROXY pointed at a dead port → full rebuild succeeded entirely from cache.
3. Content spot-checks: released sun55i-a527-cubie-a5e.dtb contains "Radxa Cubie A5E" / "radxa,cubie-a5e"; bytes at offset 8192 of hello-cubie-a5e.img compare EQUAL to the release's u-boot-sunxi-with-spl.bin (819,137 bytes) — the sunxi raw write lands where the board profile promises.

Note for future activations: the activation subagent stalled by parking its verification builds in ITS OWN background (killed at turn end — the known pattern); the builds above were re-run from the orchestrator session.
