---
# gosd-z9l4
title: Remove legacy g_mass_storage gadget from Rockchip/qemu kernel configs
status: completed
type: bug
priority: normal
created_at: 2026-07-23T11:56:48Z
updated_at: 2026-07-23T14:49:48Z
---

Found during first real-hardware boot (gosd-sz6p, rock-4se, 2026-07-23): kernel logs show g_mass_storage probing at boot and failing — 'no file given for LUN0', 'udc fe800000.usb: failed to start g_mass_storage: -22'.

Cause: CONFIG_USB_MASS_STORAGE=y (the LEGACY mass-storage gadget, drivers/usb/gadget/legacy/) is built into four boards' kernels: rock-4se, nanopi-zero2, radxa-zero-3e, qemu-virt (the Pi boards don't have it). Built-in legacy gadgets auto-bind the UDC at boot; besides the boot noise this can claim the UDC and conflict with the configfs-based gadget stack (gadget/ library) that GoSD actually uses — we already carry CONFIG_USB_CONFIGFS_MASS_STORAGE=y for that path.

Fix: unset CONFIG_USB_MASS_STORAGE in all four kernel configs (via kernelspec fragments as appropriate). This is a compiled-artifact change: fleet kernel rebuild + the artifacts tag-first release dance (docs/artifacts.md) — do NOT bump internal/artifacts.Version in the same PR.

Verify: boot log free of g_mass_storage errors on rock-4se serial + qemu boot-to-HTTP CI; USB gadget examples (usbserial/usbwebsite) still enumerate.

Also fold into this PR (it already rebuilds the rock-4se kernel artifacts): update build/boards/rock-4se/kernel/patches/0003-usb-dwc3-peripheral.patch's 'WHICH PHYSICAL PORT (UNRESOLVED, best guess)' comment — hardware-verified 2026-07-23 (gosd-sz6p): usbdrd_dwc3_0 IS the top/upper blue USB 3.0 port; CDC-ACM gadget enumerated and echoed on macOS. The unmarked OTG switch's away-from-Ethernet position = device mode. Comment-only change to the patch (DTB output unchanged), safe to ship with the config fix.

## Summary of Changes

Added `# CONFIG_USB_MASS_STORAGE is not set` to all four affected fragments (rock-4se, radxa-zero-3e, nanopi-zero2, qemu-virt kernel-fragment.config), each with a comment in that fragment's established style explaining the auto-bind/UDC-contention risk and pointing at this bean. Added `CONFIG_USB_MASS_STORAGE` to qemu-virt's `ForbiddenY` in internal/kernelspec/kernelspec.go (its documented purpose: asserting unwanted real-hardware/legacy options stay cut). Did not touch any kernel.config (build outputs) or bump internal/artifacts.Version.

Confirmed before editing: (1) radxa-zero-3e's fragment header states fragments are merged onto `make ARCH=arm64 defconfig` via `scripts/kconfig/merge_config.sh` then finalized with `make olddefconfig`, with the resulting full config "committed as kernel.config" — i.e. kernel.config is a generated artifact, not hand-authored; docs/artifacts.md confirms the same (kernel.config is written by `gosd build-kernel --staging` and packaged into the release tarball). (2) merge_config.sh treats `# CONFIG_X is not set` fragment lines as authoritative disables — verified empirically, not just by reading the tool: the existing `# CONFIG_WLAN is not set` lines in radxa-zero-3e's and qemu-virt's fragments correctly show up as `# CONFIG_WLAN is not set` in their committed kernel.config, the identical mechanism CONFIG_USB_MASS_STORAGE now goes through. kernel.config files will pick up the change at the next real `gosd build-kernel` run / artifacts release, per the tag-first rule.

kernelspec_test.go needed no changes: reviewed the board-count list (allBoardIDs), the Rockchip DTS-patch allowlist (TestDTSPatchesOnlyOnRockchipBoards), and the outputs-vs-Artifacts map (TestKernelSpecOutputsMatchBoardArtifacts) — none enumerate ForbiddenY or fragment content beyond the Pi-only RequiredY-derivation test, so none needed updating for this change.

Task 2: rewrote the 0003-usb-dwc3-peripheral.patch 'WHICH PHYSICAL PORT (UNRESOLVED, best guess)' comment block with the hardware-verified facts (usbdrd_dwc3_0 = top/upper blue USB 3.0 port; CDC-ACM echo round-trip; OTG switch position away from Ethernet jack = device mode). The new paragraph is 2 lines shorter than the old one, so the hunk header was updated from `@@ -105,3 +105,31 @@` to `@@ -105,3 +105,29 @@` to keep the diff's line-count bookkeeping correct. Verified the patch actually applies: built a synthetic base .dts file reproducing the 3 unchanged context lines at the correct 105-107 line offset and ran `patch -p1 --dry-run --forward` (clean) then a real apply (correct resulting file content) — not just a visual read of the hunk.

**Deviation found, not fixed here (scope discipline):** while confirming the bean's cause statement, `grep CONFIG_USB_MASS_STORAGE` on the committed kernel.config shows pi-zero-2w and pi-zero-w ALSO carry `CONFIG_USB_MASS_STORAGE=y` (both have CONFIG_USB_GADGET=y DWC2 dual-role fragments, so the legacy gadget Kconfig dependency is satisfied there too) — this contradicts this bean's own cause paragraph ("the Pi boards don't have it"). This PR's explicit scope is the four Rockchip/qemu boards only, so the Pi fragments were left untouched; flagging here per the repo's locked-decision-diverges-stop-and-say-so convention rather than silently expanding this PR's diff. Recommend a small follow-up bean for the two Pi boards.

Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .` (clean), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` (0 issues both) all pass.

PR: https://github.com/jphastings/gosd/pull/96 (stacked on #94). Build-verification workflow_dispatch run: https://github.com/jphastings/gosd/actions/runs/30017592659 (not waited on).
