---
# gosd-f59k
title: Pi images omit the DTB from the boot partition
status: completed
type: bug
priority: normal
created_at: 2026-07-24T19:18:28Z
updated_at: 2026-07-25T06:00:20Z
---

Found during Pi Zero 2W first hardware boot (gosd-m9dj, 2026-07-24). The assembled image's GOSD-BOOT partition contained firmware (bootcode.bin/start.elf/fixup.dat), kernel8.img, initramfs, config.txt, cmdline.txt, gosd.toml — but NO bcm2710-rpi-zero-2-w.dtb, even though the v0.6.0 artifact tarball ships it. Result: firmware boots, kernel8.img loads, and the kernel hangs before any console output (no device tree = no drivers, no UART) — a completely silent failure with healthy-looking ACT-LED activity. Diagnosed by inspecting the flashed card; hand-copying the DTB from the artifact cache onto the partition fixed boot immediately (bench-verified).

Fix: the Pi board assembly path (internal/pipeline + pi board profiles' boot-file lists) must copy the DTB artifact to the FAT partition, presumably for both pi-zero-2w and pi-zero-w (check pi-zero-w's assembly for the same omission — its DTB name differs, bcm2708-rpi-zero-w.dtb or similar per its manifest). Add an integration-test assertion (the build_integration_test.go pattern reads built images back) that each Pi board's boot partition contains its DTB file — the exact assertion that would have caught this. Rockchip boards are unaffected (extlinux fdt line + verified on hardware).

How this survived CI: qemu-virt exercises a different boot path entirely, and no fixture test asserted Pi boot-partition completeness against the artifact manifest.

## Summary of Changes

Root cause was `pi-zero-2w` only: its board profile (`internal/boards/pizero2w/board.go`) never declared a DTB artifact at all, unlike `pi-zero-w` (`internal/boards/pizerow/board.go`), which already resolves and copies `bcm2835-rpi-zero-w.dtb` correctly. So only one board needed fixing.

- `internal/boards/pizero2w/board.go`: added `dtbArtifactName = "bcm2710-rpi-zero-2-w.dtb"` (matching the filename `internal/kernelspec` builds and the artifact release tarball ships), added it to `Artifacts()` so the pipeline resolves it from `--artifacts-dir` or the CI-built artifact release, and added it to `BootFiles()` so it's copied onto the FAT boot partition — mirroring `pizerow`'s existing pattern exactly.
- `internal/boards/pizero2w/board_test.go`: extended `TestArtifactsIncludesKernelAndManifestFiles` (renamed `TestArtifactsIncludesKernelDTBAndManifestFiles`) and `TestBootFilesContents` to assert the DTB is present and URL-less, mirroring `pizerow/board_test.go`.
- `cmd/gosd/testdata/fake-artifacts/bcm2710-rpi-zero-2-w.dtb`: new fixture file (the fake-artifacts dir had every other board's DTB already, just not this one — proof the gap was never exercised).
- `cmd/gosd/build_integration_test.go`: `TestBuildProducesABootableImageFromFakeArtifacts` now asserts `bcm2710-rpi-zero-2-w.dtb` is present in the built image's boot partition — the assertion that would have caught this, matching the equivalent assertion the pi-zero-w test already had.
- `COMPATIBILITY.md`: added a `[^pi-dtb]` footnote on the Pi Zero 2W's "Image build via `gosd build`" cell documenting the bug and the fix, following the same pattern as the existing `[^radxa-serial]` hardware-bring-up footnote.
- `internal/kernelspec/kernelspec_test.go`: this exact gap was already flagged, as a documented exemption, by `TestKernelSpecOutputsMatchBoardArtifacts` (bean `gosd-di6v`) — `dtbExemptFromArtifacts["pi-zero-2w"]` existed because the drift guard caught pi-zero-2w's `KernelSpec.DTB.Filename` not appearing in its `Board.Artifacts()`, but fixing the wiring was out of that bean's scope. Removed the now-stale exemption so the drift guard actually re-engages for this board.

Rockchip boards were out of scope and untouched — they place their DTB via an extlinux `fdt` line, a different mechanism already verified on hardware.

Hardware verification that a real flash now boots without the hand-patch (previously required on every pi-zero-2w flash) remains a bench follow-up under `gosd-m9dj`.
