---
# gosd-u5yz
title: 'Pi CM4: trimmed arm64 kernel build (kernel8.img) + CI artifacts job'
status: todo
type: task
created_at: 2026-08-30T10:25:55Z
updated_at: 2026-08-30T10:25:55Z
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
