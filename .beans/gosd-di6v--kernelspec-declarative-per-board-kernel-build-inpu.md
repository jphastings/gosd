---
# gosd-di6v
title: 'KernelSpec: declarative per-board kernel build inputs in Go'
status: todo
type: task
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
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
    note `go:embed` cannot reach outside the package dir, so fragments/patches
    physically move under the owning Go package; `build/boards/` retains only
    what U-Boot builds still need
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

- [ ] Define `KernelSpec` type + per-board specs (all five kernel-building boards)
- [ ] Move fragments/patches under the owning package for `go:embed`; update any references (README paths, docs)
- [ ] Drift test: KernelSpec output names match board `ArtifactRef.Name`s
- [ ] Quality gates green (incl. `GOOS=linux golangci-lint`)
