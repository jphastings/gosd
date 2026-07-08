---
# gosd-xshg
title: Pin artifacts version to v0.3.0 (I2C-enabled Rockchip DTBs)
status: completed
type: task
priority: normal
created_at: 2026-07-08T02:50:31Z
updated_at: 2026-07-08T02:59:04Z
parent: gosd-85pt
---

Tag-first follow-up for bean gosd-85pt: artifacts/v0.3.0 is published (Radxa Zero 3E i2c3 + NanoPi Zero2 i2c5 DTS patches), bump internal/artifacts.Version from v0.2.0 to v0.3.0 so real gosd build runs pick it up. Verify with a clean-machine + offline acceptance run and a DTB decompile spot-check on both Rockchip boards.


## Summary of Changes

Bumped `internal/artifacts.Version` from `v0.2.0` to `v0.3.0` in
`internal/artifacts/artifacts.go`, updating the doc comment to describe the
I2C-DTB refresh (Radxa Zero 3E `i2c3`, NanoPi Zero2 `i2c5`) instead of the
stale nanopi-public-flip note. No other docs named the current artifact
version as a fact that needed updating (the two remaining `v0.2.0` mentions
in `cmd/gosd/build.go` and the Radxa kernel README are historical —
"published in v0.2.0" / "same dance as v0.2.0" — and remain accurate).

Acceptance run (fresh `HOME`, no `--board`/`--artifacts-dir`):
`gosd build ./examples/hello` downloaded and sha256-verified
`artifacts/v0.3.0` and produced all four public board images
(`pi-zero-2w`, `pi-zero-w`, `radxa-zero-3e`, `nanopi-zero2`, ~1.27 GiB each,
qemu-virt correctly excluded from the default set). A second run with a
dead `HTTPS_PROXY` and a fresh output dir, same `HOME`, succeeded fully
offline from the populated cache. `os.UserCacheDir()` was independently
confirmed to resolve under the fresh `HOME` (`$HOME/Library/Caches` on
macOS).

Decompiled both downloaded Rockchip DTBs with `dtc -I dtb -O dts`:
`rk3566-radxa-zero-3e.dtb`'s `i2c3` node (`i2c@fe5c0000`, aliased via
`i2c3 = "/i2c@fe5c0000"`) has `status = "okay"` with
`pinctrl-0` resolving to `i2c3m0-xfer`; `rk3528-nanopi-zero2.dtb`'s `i2c5`
node (`i2c@ffa78000`) likewise has `status = "okay"` with `pinctrl-0`
resolving to `i2c5m0-xfer`. Confirms the published release's DTBs actually
carry gosd-85pt's DTS patch, not just that the tarballs downloaded.

All quality gates green: `go test ./...`, `go vet ./...`, `gofmt -l .`
(empty), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`
(0 issues both).
