---
# gosd-di6v
title: 'KernelSpec: declarative per-board kernel build inputs in Go'
status: completed
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T08:27:33Z
parent: gosd-47rm
---

Foundation bean for the [[gosd-47rm]] epic: make the Go board model the single
source of truth for how each board's kernel is built, replacing the values
currently scattered across `build/boards/<board>/**/build.sh`,
`docker-build.sh`, and `.github/workflows/build-artifacts.yml`.

## Locked decisions

- A declarative **`KernelSpec`** per board, covering everything the shell
  scripts pin today:
  - source repo + ref (`raspberrypi/linux` @ commit `63598c83…` for both Pi
    boards, incl. `COMMIT_DATE`; kernel.org stable @ fleet tag `v6.18.37` for
    radxa-zero-3e / nanopi-zero2 / qemu-virt)
  - defconfig name (`bcm2711_defconfig`, `bcmrpi_defconfig`, arm64 `defconfig`)
  - the GoSD config fragment and DTS patches, **embedded via `//go:embed`** —
    note `go:embed` cannot reach outside the package dir. **Refined during
    implementation:** rather than physically moving fragments/patches, each
    `build/boards/<board>/` (or its `kernel/` subdir) hosts the embed *in
    place* — `pi-zero-2w`/`pi-zero-w` already had a Go package there
    (`manifest`), so a new file was added to it; the three Rockchip-family
    dirs (`radxa-zero-3e/kernel`, `nanopi-zero2/kernel`, `qemu-virt/kernel`)
    got a new small `kernelassets` package. No fragment/patch file moved, so
    every `build.sh`/`docker-build.sh` keeps reading them unchanged from
    disk. Reason: those scripts are still the only thing that actually builds
    a kernel until gosd-07fl retires them — moving the files would have broken
    them mid-epic.
  - DTB make-target + output DTB filename, kernel output filename — these must
    equal the board's existing `ArtifactRef.Name`s (`kernel8.img`,
    `kernel.img`, `Image`, `*.dtb`); add a test asserting KernelSpec output
    names ⊆ `Artifacts()` names so the two can't drift
  - cross-toolchain triple (`aarch64-linux-gnu-` / `arm-linux-gnueabihf-`)
  - the required-`=y` assertion list (from the Rockchip `docker-build.sh`
    `required_y` arrays; derive equivalents for the Pi boards from their
    fragments)
  - reproducibility pins: `KBUILD_BUILD_TIMESTAMP` (= pinned commit date),
    `KBUILD_BUILD_USER=gosd`, `KBUILD_BUILD_HOST=gosd-ci` — required for the
    byte-identity gate in the CI dogfood bean
- **qemu-virt has a KernelSpec too** (CI builds its kernel); internal-only
  status is unchanged.
- The committed generated `kernel.config` files stay committed (comparison /
  review aids), wherever the fragments end up living.
- Exposure: keep the public `Board` interface unchanged if practical — a
  lookup by board ID (e.g. in a new `internal/` package) is fine; only
  `build-kernel` needs it.
- Behavioral tests on macOS, no Docker: spec resolution per board, name-drift
  assertion, fragment/patch embedding non-empty.

## Todos

- [x] Define `KernelSpec` type + per-board specs (all five kernel-building boards)
- [x] Embed fragments/patches in place via `go:embed` in each board's owning package (no files moved; see refined locked decision above)
- [x] Drift test: KernelSpec output names match board `ArtifactRef.Name`s (see `TestKernelSpecOutputsMatchBoardArtifacts` in `internal/kernelspec/kernelspec_test.go`; documents one pre-existing gap — pi-zero-2w's DTB isn't in its `Artifacts()` — as an explicit exemption rather than fixing board wiring, which is out of scope here)
- [x] Quality gates green (incl. `GOOS=linux golangci-lint`)

## Summary of Changes

Added `internal/kernelspec`, a Go-native `KernelSpec` type + a registry
(`Get(boardID)`, `BoardIDs()`) covering all five kernel-building boards
(pi-zero-2w, pi-zero-w, radxa-zero-3e, nanopi-zero2, qemu-virt). Each
`KernelSpec` captures: source repo/ref (+ commit date where pinned),
defconfig, toolchain (`ARCH`/`CROSS_COMPILE`), the embedded Kconfig
fragment, ordered DTS patches (Rockchip-family only), kernel/DTB make
targets + source paths + output filenames, the required-`=y` (and, for
qemu-virt, forbidden-`=y`) assertion lists, `ModulesDisabled`, and
reproducibility pins (`KBUILD_BUILD_TIMESTAMP/USER/HOST`).

Fragments/patches are embedded **in place** (see the refined locked
decision above): `build/boards/pi-zero-2w` and `build/boards/pi-zero-w`
gained a `kernelfragment.go` file in their existing `manifest` package;
`build/boards/{radxa-zero-3e,nanopi-zero2,qemu-virt}/kernel` each gained a
new `kernelassets` package. No existing file moved or was modified, so
every `build.sh`/`docker-build.sh` still works unchanged.

The Pi boards' `RequiredY` is *derived*, not copied: `requiredYFromFragment`
extracts every literal `CONFIG_*=y` line from the embedded fragment, so it
can't drift from `kernel.fragment` by hand-editing one and not the other.
The Rockchip-family boards' `RequiredY`/`ForbiddenY` are copied from each
`docker-build.sh`'s bash arrays (those aren't mechanically derivable from
the fragment — they're a curated subset); `TestRockchipRequiredYMatchesScript`
parses the actual scripts and fails if the copies drift, which is the
stopgap this bean's design doc calls for pending gosd-07fl.

`TestKernelSpecOutputsMatchBoardArtifacts` is the drift guard: every
`KernelSpec` output filename must be one of the board's
`Board.Artifacts()` `ArtifactRef.Name`s. This surfaced one pre-existing
gap, not introduced by this bean and left unfixed (out of scope): pi-zero-2w's
`build.sh` produces `bcm2710-rpi-zero-2-w.dtb`, but
`internal/boards/pizero2w`'s `Artifacts()`/`BootFiles()` never asks for a
DTB artifact (unlike pi-zero-w, which does) — the GPU firmware's own
fallback device tree is used instead. The spec still records the DTB
faithfully (it's what the script produces); the drift test carries an
explicit, documented exemption for this one board/field rather than
silently omitting it. Worth a follow-up bean if pi-zero-2w ever needs its
own DTB on the SD card (e.g. once DTS patches are needed for it).

No shell script, README, `COMPATIBILITY.md`, workflow file, or
`internal/artifacts.Version` was touched — this bean is purely additive.
