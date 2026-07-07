---
# gosd-et0q
title: 'Pi Zero W board profile: registry, arch, catalog tag'
status: completed
type: task
priority: normal
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-07T14:39:55Z
parent: gosd-ajpz
blocked_by:
    - gosd-2j6z
    - gosd-s7fk
    - gosd-06kj
---

Wire it together: internal/boards/pizerow profile (Artifacts: kernel.img + bcm2835-rpi-zero-w.dtb; BootFiles incl. rendered templates + initramfs; FirmwareFiles from the 43430 manifest incl. aliases; no RawWrites; Arch = arm/GOARM=6), registered PUBLIC. Imager catalog device tag: verify the official tag for Pi Zero/Zero W (expect pi1-32bit family) from the v4 os_list like the pi3-64bit fix did — cite evidence. Integration test: fake-artifacts build produces an image whose /app and /init are 32-bit ARM ELFs; no---board now emits three public images. COMPATIBILITY.md column + footnotes in the same PR (per convention), incl. the armv6/performance caveat.
- [x] Profile + registration + integration tests
- [x] Catalog tag verified + golden tests
- [x] COMPATIBILITY.md updated

## Summary of Changes

- Added `internal/boards/pizerow` (mirrors `pizero2w`): `Arch()` returns
  `{GOARCH: "arm", GOARM: "6"}`; `Artifacts()` is `kernel.img` +
  `bcm2835-rpi-zero-w.dtb` (both fetched via --artifacts-dir/CI-artifact
  fallback, no pinned URL) plus the shared GPU boot firmware and the three
  43430 WiFi files from `build/boards/pi-zero-w/manifest.json`; `BootFiles`
  renders the already-locked `pizerow/templates` config.txt/cmdline.txt;
  `RawWrites` is nil (GPU-ROM boot, no bootloader); `FirmwareFiles` flattens
  the three Cypress-blob aliases into `brcm/` as literal duplicate copies,
  per gosd-06kj's finding that the Zero W's "43430" alias is different bytes
  from the Zero 2W's. Added `build/boards/pi-zero-w/manifest.go` to embed
  that manifest (mirrors pi-zero-2w's embed package).
- Registered `pi-zero-w` as a PUBLIC board in `cmd/gosd/build.go`.
- `internal/catalog`: added `pi-zero-w` -> "Raspberry Pi Zero W" to
  `boardDisplayNames`, and `pi-zero-w` -> `["pi1-32bit"]` to
  `boardImagerDeviceTags`. Verified directly against
  `downloads.raspberrypi.org/os_list_imagingutility_v4.json` (fetched
  2026-07-07): the "Raspberry Pi Zero" device entry (description literally
  "Raspberry Pi Zero, Zero W, and Zero WH") carries tags `["pi1-32bit"]` —
  confirming the expected pi1-32bit family. Also discovered (and documented
  in code + COMPATIBILITY.md) that "Raspberry Pi 1" carries the identical
  `["pi1-32bit"]` tag, so — the same shared-namespace side effect as
  pi-zero-2w/Pi 3 — a GoSD Pi Zero W catalog entry will also surface when a
  user selects "Raspberry Pi 1" in Imager. Extended catalog_test.go (device
  tag + display name cases) and regenerated the golden `os_list.json`
  fixture with a third (pi-zero-w) entry.
- `internal/artifacts`: bumped `Version` to `v0.2.0` (comment updated to
  note this release doesn't exist until the next `artifacts/v0.2.0` tag
  push, and that it will be the first release containing the pi-zero-w and
  nanopi-zero2 kernels). No other change was needed there: `EnsureBoard`
  already resolves a board's tarball name generically from the board string
  passed in (`<board>.tar.zst`), so pi-zero-w needed no separate
  registration — a deviation from the task's phrasing worth flagging.
- `cmd/gosd/build_integration_test.go`: added
  `TestBuildProducesABootableImageForPiZeroWFromFakeArtifacts`, seeding new
  fake artifacts (`kernel.img`, `bcm2835-rpi-zero-w.dtb`,
  `cyfmac43430-sdio.{bin,clm_blob}`, `brcmfmac43430-sdio.txt`) under
  `cmd/gosd/testdata/fake-artifacts`. Unlike the other boards' fake-artifact
  tests, `/app` and `/init` are NOT fakes here — the pipeline really
  cross-compiles `examples/hello` and `gosd-init` for `GOARM=6`, and the
  test parses them out of the initramfs with `debug/elf` and asserts
  `ELFCLASS32`/`EM_ARM`, closing the loop on gosd-2j6z's multi-arch build
  work. Also confirms config.txt has no `arm_64bit` line and
  `kernel=kernel.img`, and that all three WiFi aliases are present. Updated
  `TestBuildWithNoBoardFlagBuildsAllBoards` to expect three public images
  (pi-zero-2w, pi-zero-w, radxa-zero-3e), still excluding qemu-virt.
- `COMPATIBILITY.md`: added the Raspberry Pi Zero W column with a new
  `[^armv6-perf]` footnote (single ARM1176JZF-S core, no NEON, real
  performance ceiling vs. the Zero 2W's quad-core arm64), a
  `[^pi-zero-w-wifi]` footnote on the Cypress-blob WiFi firmware, and the
  `[^pi-zero-w-tag]` catalog-tag footnote with the same evidence/citation
  style as the existing `[^pi-tag]` (pi-zero-2w) entry. Existing multi-board
  footnotes (`[^pi-no-eth]`, `[^usb-gadget]`, `[^gpio]`) were reworded to
  cover all three boards instead of just two.

All quality gates pass: `go test ./...`, `go vet ./...`, `gofmt -l .`
(clean), and `golangci-lint run ./...` both natively and with
`GOOS=linux`.
