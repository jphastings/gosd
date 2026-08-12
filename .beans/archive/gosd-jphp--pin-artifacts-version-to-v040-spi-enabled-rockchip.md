---
# gosd-jphp
title: Pin artifacts version to v0.4.0 (SPI-enabled Rockchip DTBs)
status: completed
type: task
priority: normal
created_at: 2026-07-09T20:06:41Z
updated_at: 2026-07-09T20:09:35Z
parent: gosd-fnza
---

Tag-first follow-up for bean gosd-fnza: artifacts/v0.4.0 is published (Radxa Zero 3E spi3 + NanoPi Zero2 spi1 DTS patches, each with a spidev child node). Bump internal/artifacts.Version from v0.3.0 to v0.4.0 so real gosd build runs pick it up. Verify with a clean-machine + offline acceptance run and a DTB decompile spot-check on both Rockchip boards.



## Summary of Changes

Bumped `internal/artifacts.Version` from `v0.3.0` to `v0.4.0` in `internal/artifacts/artifacts.go`, updating the doc comment to describe the SPI-DTB refresh (Radxa Zero 3E `spi3` + spidev node, NanoPi Zero2 `spi1` + alias + two spidev nodes) instead of the stale I2C-only note. No docs/README named the current pinned artifact version as a fact needing an update: the `docs/artifacts.md` cache-path example uses `v0.1.0` illustratively, and the two remaining older-version mentions (`cmd/gosd/build.go` 'published in v0.2.0', Radxa kernel README 'same dance as v0.2.0') are historical and remain accurate.

Real-release acceptance run (fresh `HOME`, no `--board`/`--artifacts-dir`): `gosd build ./examples/hello` downloaded and sha256-verified `artifacts/v0.4.0` and produced all four public board images (`pi-zero-2w`, `pi-zero-w`, `radxa-zero-3e`, `nanopi-zero2`, ~1.27 GiB each; qemu-virt correctly excluded), ~2m32s. `os.UserCacheDir()` independently confirmed to resolve under the fresh `HOME` (`$HOME/Library/Caches` on macOS). Offline re-run with a dead `HTTPS_PROXY`/`HTTP_PROXY` (127.0.0.1:1) and a fresh output dir, same `HOME`: succeeded fully from the populated cache in ~18s, no network touched.

SPI-DTB spot-check (the point of v0.4.0) — decompiled the DTBs the build actually pulled from the real release with `dtc -I dtb -O dts`:
- **radxa-zero-3e** `rk3566-radxa-zero-3e.dtb`: `spi@fe640000` (aliased `spi3 = /spi@fe640000`) has `status = "okay"` with a `spidev@0` child, `compatible = "rohm,dh2228fv"`.
- **nanopi-zero2** `rk3528-nanopi-zero2.dtb`: `spi@ff9d0000` (aliased `spi1 = /soc/spi@ff9d0000`) has `status = "okay"` with two children `spidev@0` and `spidev@1`, both `compatible = "rohm,dh2228fv"`.
This confirms the published release's DTBs actually carry gosd-fnza's SPI DTS patch, not just that the tarballs downloaded. Pi half rode along: the `pi-zero-2w` image's `config.txt` (extracted via `imgextract`) contains `dtparam=spi=on`.

All quality gates green: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` (0 issues both).
