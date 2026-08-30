---
# gosd-u5yz
title: 'Pi CM4: trimmed arm64 kernel build (kernel8.img) + CI artifacts job'
status: completed
type: task
priority: normal
created_at: 2026-08-30T10:25:55Z
updated_at: 2026-08-30T11:08:16Z
parent: gosd-7676
---

## What

Run `gosd build-kernel --board pi-cm4` for real (Docker-backed,
20-60 min — run backgrounded from the orchestrating session, never a
subagent's background task), commit the real `kernel.config` under
`build/boards/pi-cm4/`, and add `internal/kernelspec/kernelconfigsnapshot_test.go`'s
`pi-cm4` entry.

Add the `pi-cm4-kernel` job to `.github/workflows/build-artifacts.yml`,
mirroring the other Pi kernel jobs (stage kernel8.img, bcm2711-rpi-cm4.dtb,
kernel.config, source.json under staging/pi-cm4). Board stays
internal-only; activation is the separate artifacts-release bean.


## Summary of Changes

Real `gosd build-kernel --board pi-cm4` run completed successfully
(Docker-backed, ~25 min). Committed `build/boards/pi-cm4/kernel.config`
(the actual build output) and added its `kernelConfigSnapshotPath` entry;
`TestKernelConfigSnapshotMatchesAssertions` passes with zero drift against
the fragment's RequiredY/ForbiddenY/ModulesDisabled assertions. Spot-checked
the storage-controller choice (the one genuinely risky call in the
fragment): `CONFIG_MMC_SDHCI_IPROC=y` present as expected, alongside
`CONFIG_BCMGENET=y`, `CONFIG_USB_DWC2_DUAL_ROLE=y`, and
`# CONFIG_MODULES is not set`; `CONFIG_BRCMFMAC`/`CONFIG_BT`/
`CONFIG_MAC80211_HWSIM` all confirmed "is not set".

Added the `pi-cm4-kernel` CI job to `.github/workflows/build-artifacts.yml`
(mirrors pi-3b-kernel's shape), wired into `package-and-release`'s
needs/download-artifact steps, deliberately NOT yet in the public release
upload list (same internal-only treatment as turing-rk1, until gosd-6hdc).

All quality gates green: go build/vet/test, gofmt, golangci-lint (native +
GOOS=linux).
