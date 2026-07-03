---
# gosd-y0x3
title: Artifact pipeline, CI & docs
status: todo
type: epic
created_at: 2026-07-02T20:50:26Z
updated_at: 2026-07-02T20:50:26Z
parent: gosd-cij4
---

Kernels and bootloaders are prebuilt in GitHub Actions, published as versioned GitHub Releases with sha256 checksums, and downloaded+cached by the CLI (~/.cache/gosd). Go developers never compile a kernel; CI pipelines just 'go run gosd build'.

Also: repo CI (test/lint/build example images on PR) and the two docs audiences — Go developers (quickstart) and end users (flash guide with screenshots).
