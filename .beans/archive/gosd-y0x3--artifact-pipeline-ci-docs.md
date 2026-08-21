---
# gosd-y0x3
title: Artifact pipeline, CI & docs
status: completed
type: epic
priority: normal
created_at: 2026-07-02T20:50:26Z
updated_at: 2026-08-21T01:36:31Z
parent: gosd-cij4
---

Kernels and bootloaders are prebuilt in GitHub Actions, published as versioned GitHub Releases with sha256 checksums, and downloaded+cached by the CLI (~/.cache/gosd). Go developers never compile a kernel; CI pipelines just 'go run gosd build'.

Also: repo CI (test/lint/build example images on PR) and the two docs audiences — Go developers (quickstart) and end users (flash guide with screenshots).

## Summary of Changes

The promise held: a Go developer never compiles a kernel. gosd-wtpa built the
artifact pipeline — GitHub Actions compiles each board's kernel and U-Boot,
publishes them as an `artifacts/vX.Y.Z` GitHub Release with per-file sha256
digests in a `manifest.json`, and the CLI downloads and caches them under the
OS user cache dir; `internal/artifacts` pins the version, and the manifest's
own digest beside it is the trust anchor every per-file digest is read
through. gosd-2ge8 built repo CI — test, vet, gofmt, lint and real image
smoke-builds on every PR — which has since grown the qemu boot-to-HTTP and
data-partition jobs. gosd-mr2n wrote the two docs audiences: the README
quickstart and the runtime contract for Go developers, and (with gosd-ufeh)
the screenshot-driven flash guide for end users.

Everything since has been iteration on this base rather than new ground:
tag-first/bump-second artifact releases (docs/artifacts.md), knope-driven
release automation (gosd-vt2l), and cache pruning bounded to the current pins
(gosd-gdro).
