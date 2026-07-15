---
# gosd-iosp
title: 'ROCK 4SE: trimmed mainline kernel build'
status: in-progress
type: task
priority: normal
created_at: 2026-07-13T12:25:00Z
updated_at: 2026-07-15T19:39:22Z
parent: gosd-cuym
blocked_by:
    - gosd-je2r
---

Mirror `build/boards/radxa-zero-3e/kernel/` + its kernelspec entry for rock-4se: `kernelassets.go` (//go:embed), `kernel-fragment.config`, `patches/`, committed generated `kernel.config` (GPL provenance header), README; new entry in `internal/kernelspec/kernelspec.go` (fleet repo/tag v6.18.37, `defconfig`, arm64 toolchain, DTB target `rockchip/rk3399-rock-4se.dtb`, `KernelFilename: "Image"`, ModulesDisabled). Adds the rock-4se kernel job to `.github/workflows/build-artifacts.yml` (no artifacts.Version bump here — tag-first, bump-second).

## Locked decisions

- Fragment beyond the Rockchip trim baseline: `CONFIG_PCI`/`CONFIG_PCIE_ROCKCHIP_HOST`/`CONFIG_PHY_ROCKCHIP_PCIE`, `CONFIG_BLK_DEV_NVME`, `CONFIG_EXFAT_FS` (+NLS deps) — the betamin SSD is exFAT (host-native for USB mass-storage exposure).
- USB gadget: mirror radxa-zero-3e's gadget enablement (dwc3 dual-role, configfs) **plus `CONFIG_USB_CONFIGFS_MASS_STORAGE`**.
- RK3399-specifics from A1's findings: SD controller driver, USB PHYs (INNO_USB2, Type-C), GbE (DWMAC_ROCKCHIP + PHY).
- DRM/SOUND/WLAN/MEDIA stay `is not set` (fleet trim policy; video is developer-recipe territory).
- DTS patches: `status="okay"` for header I2C bus(es) + header SPI with spidev child node (accepted compatible), per the per-SoC peripheral convention; dwc3 dr_mode patch if A1 found host-only.
- RequiredY mirrors the fragment's additions (hand-maintained literals, nanopi style).
- `TestKernelSpecOutputsMatchBoardArtifacts` binds artifact names to the A4 board profile — coordinate names with A4.

## Todo

- [x] kernelspec entry + build/boards/rock-4se/kernel/ scaffolding
- [x] Fragment + RequiredY (baseline trim + NVMe/exFAT/gadget/mass-storage + A1 specifics)
- [x] DTS patches (header I2C, header SPI+spidev, dr_mode if needed); confirm each applies at the pinned tag
- [ ] Real `gosd build-kernel --board rock-4se` Docker build; commit generated kernel.config; spot-check symbols =y
- [ ] CI kernel job in build-artifacts.yml (blocked: needs gosd-0vvh's board registration first - see Scaffolding status)

## Scaffolding status

Non-Docker scaffolding pass complete (branch `bean/gosd-iosp-rock4se-kernel`,
pushed, no PR yet - a human reviews the kernel config before the hour-long
Docker build runs). Bean kept `in-progress`.

**Done, reviewable now:**
- `build/boards/rock-4se/kernel/kernelassets.go`, `kernel-fragment.config`,
  `patches/000{1,2,3}-*.patch`, `README.md`.
- `internal/kernelspec/kernelspec.go`'s new `"rock-4se"` entry (fleet tag
  v6.18.37, `defconfig`, arm64 toolchain, DTB target
  `rockchip/rk3399-rock-4se.dtb`, `KernelFilename: "Image"`,
  `ModulesDisabled: true`, full `RequiredY`).
- Every new `CONFIG_*` symbol was checked against the actual pinned kernel
  tree's Kconfig files at v6.18.37 (git.kernel.org), not assumed from docs -
  notably confirmed `CONFIG_PHY_ROCKCHIP_TYPEC` exists and
  `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY` does not apply here (RK3566-only,
  per gosd-je2r).
- All three DTS patches were generated from a real diff against
  `rk3399-rock-4se.dts` fetched at tag v6.18.37, then test-applied
  sequentially with real GNU patch (`patch -p1 --forward --fuzz=0`,
  installed locally via `gpatch` to match the Debian container's `patch`
  package) against that exact source - all three applied cleanly with zero
  fuzz/offset, output byte-identical to the intended result.

**Cross-bean test coupling found (not worked around):** adding the
`"rock-4se"` kernelspec entry makes `kernelspec.BoardIDs()` return 6 boards,
which fails `TestBoardIDsListsExactlyTheFiveKernelBuildingBoards` in
`internal/kernelspec/kernelspec_test.go` (it hardcodes the 5 current board
IDs and asserts an exact-length match). This is expected until gosd-0vvh
registers the board in `internal/boards` and that test's `allBoardIDs` list
is updated to include `rock-4se` - left failing/uncorrected here per this
bean's brief, rather than editing the test or faking a board registration.
Every other kernelspec test passes (they iterate the hardcoded board list,
not `BoardIDs()`, so they don't touch `rock-4se`).

**CI job intentionally NOT added:** `gosd build-kernel --board <id>`
resolves boards via `internal/boards.Find`, not just `internal/kernelspec` -
confirmed locally: `go run ./cmd/gosd build-kernel --board rock-4se` fails
immediately with `unknown board "rock-4se"; try one of: nanopi-zero2,
pi-zero-2w, pi-zero-w, radxa-zero-3e`. Adding a `rock-4se-kernel` job to
`.github/workflows/build-artifacts.yml` now (even standalone, not wired into
`package-and-release`) would be a guaranteed-red job the moment anyone runs
it via `workflow_dispatch`. Left for a follow-up once gosd-0vvh's board
registration lands - noting it rather than adding a broken job, per this
bean's own guidance for CI coupling.

**Remains (needs the real Docker build):**
- `gosd build-kernel --board rock-4se` (once buildable) to actually compile,
  generate `kernel.config`, and commit it with a GPL-provenance header.
- Spot-checking the built `kernel.config` for every `RequiredY` symbol =y.
- Confirming the CI job (once addable) actually runs green.

**Two flagged uncertainties, both called out in patch/README comments too:**
- `0003-usb-dwc3-peripheral.patch` flips `usbdrd_dwc3_0` to
  `dr_mode = "peripheral"` as a **best guess** for which physical USB port is
  the hardware host/device switch OTG port - gosd-je2r's research could not
  resolve this from DTS text alone (the shared `rk3399-rock-pi-4.dtsi` treats
  both dwc3 controllers symmetrically). Needs a schematic check or a
  dmesg/lsusb bring-up check; swap to `usbdrd_dwc3_1` if wrong.
- The 40-pin header pin-number mapping (I2C pins 3/5/27/28/29/31, SPI pins
  19/21/23/24) is sourced from Radxa's own docs/wiki (gosd-je2r Finding #5),
  not an opened schematic PDF - worth a quick re-check if a peripheral
  doesn't enumerate at bring-up.
