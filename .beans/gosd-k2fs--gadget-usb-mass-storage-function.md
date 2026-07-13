---
# gosd-k2fs
title: 'gadget: USB mass-storage function'
status: todo
type: feature
created_at: 2026-07-13T13:19:54Z
updated_at: 2026-07-13T13:19:54Z
parent: gosd-jge2
---

Extend the public `gadget/` package (currently serial/Ethernet functions) with a USB mass-storage function (configfs f_mass_storage): LUN backed by a block-device path, read-only flag, removable flag. Follows the package's existing function-type shape. Driving use case: betamin exposes its unmounted NVMe SSD partition to a host computer for drag-and-drop video transfer (exFAT, host-native) — MSC mode and local mount are mutually exclusive by app design.

## Locked decisions

- Public API surface (`gadget/` is semver-relevant, v0.3) — note the minor-version implication in the PR.
- Pure logic behind the interface seam with fake-driven tests passing on macOS; real configfs writes in platform_linux.go (repo convention).
- Kernel dependency: boards need `CONFIG_USB_CONFIGFS_MASS_STORAGE=y`. rock-4se gets it in its initial kernel (epic gosd-cuym); other boards gain it at the next fleet kernel tag bump — track that follow-up here, do NOT bump a single board in isolation.
- COMPATIBILITY.md: extend the USB gadget row/footnote accordingly.

## Todo

- [ ] gadget/ mass-storage function type + configfs wiring (platform_linux.go + stubs)
- [ ] Fake-driven tests
- [ ] COMPATIBILITY.md gadget footnote update
- [ ] Follow-up note/bean for fleet kernel fragment addition at next tag bump
