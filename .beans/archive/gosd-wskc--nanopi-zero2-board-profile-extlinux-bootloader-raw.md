---
# gosd-wskc
title: 'NanoPi Zero2: board profile (extlinux + bootloader raw-writes)'
status: completed
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-07T19:37:10Z
parent: gosd-cwjf
blocked_by:
    - gosd-f39b
    - gosd-rqx8
---

Mirror internal/boards/radxazero3e: register board ID nanopi-zero2; Artifacts() = idbloader.img, u-boot.itb, Image, <board>.dtb; RawWrites at byte offsets 32768 and 8388608 with the u-boot.itb size guard; BootFiles = kernel, dtb, initramfs, extlinux/extlinux.conf (console + baud per research findings, init=/init gosd.board=nanopi-zero2); FirmwareFiles empty. Extend the fake-artifacts integration tests; no---board builds must now emit three images. Also add the board to the artifact pipeline (build-artifacts.yml + manifest) and to CLAUDE.md board IDs (already reserved) + README/docs.

## Scope amendment (2026-07-06, JP: progress as far as possible pre-hardware)
Proceed NOW without waiting for U-Boot v2026.07: implement the full profile + fake-artifact integration tests, but register via RegisterInternal (like qemu-virt) so default builds/catalog exclude it — real artifact fetches would 404 on the U-Boot files until the artifact release includes them. Add a clearly-marked TODO + a checklist item here: flip to public Register + add artifacts-pipeline U-Boot entries when gosd-f39b completes. RawWrites offsets/extlinux content: same pattern as radxa (verify DT/console details from the gosd-vcae findings: rk3528-nanopi-zero2.dtb, UART0 1500000, ttyS0).
- [x] Flip to public registration once U-Boot artifacts publish (gated on gosd-f39b)


## Summary of Changes (2026-07-07)
Implemented internal/boards/nanopizero2, mirroring internal/boards/radxazero3e:
Artifacts() = idbloader.img, u-boot.itb, Image, rk3528-nanopi-zero2.dtb (no
per-file pinned URL, same as radxa); RawWrites at offsets 32768/8388608 with
the same 16MiB u-boot.itb size guard; BootFiles renders extlinux/extlinux.conf
from a go:embed template (kernel /Image, fdt /rk3528-nanopi-zero2.dtb, initrd
/initramfs.cpio.zst, append console=ttyS0,1500000n8 quiet init=/init
gosd.board=nanopi-zero2); FirmwareFiles is empty (no runtime-loaded firmware
needed). BuildConfig.UsbGadget is ignored - no boot-time gadget change is
possible while this board has no USB controller in any numbered mainline
kernel (per gosd-cwjf's USB gate finding).

Console verification: fetched the mainline DTS at kernel tag v6.18.37
(gregkh/linux-stable, since torvalds/linux only carries release tags, not
stable point releases). rk3528-nanopi-zero2.dts's /aliases node has exactly
one serial alias, `serial0 = &uart0` (no other serialN aliases to shift
numbering); rk3528.dtsi's uart0 node is `compatible = "rockchip,rk3528-uart",
"snps,dw-apb-uart"` - the standard 8250-family driver, which enumerates as
/dev/ttySN (not the RK3288-era ttyFIQ fiq-debugger pseudo-console, which
doesn't exist on this SoC/board). serial0 -> uart0 with no other alias
therefore confirms console=ttyS0,1500000n8, matching stdout-path =
"serial0:1500000n8" in the board DT. Documented with source citations in
internal/boards/nanopizero2/templates/templates.go's doc comment.

Registered via boards.RegisterInternal (qemu-virt precedent) in
cmd/gosd/build.go, with a prominent comment marking the flip-to-Register
condition (gated on gosd-f39b publishing U-Boot artifacts). Extended
cmd/gosd/build_integration_test.go: a new fake-artifacts acceptance test for
--board=nanopi-zero2 (raw writes, boot partition contents, exact
extlinux.conf), plus updated the no---board-flag and --catalog exclusion
tests to assert nanopi-zero2 stays excluded alongside qemu-virt. Added
cmd/gosd/testdata/fake-artifacts/rk3528-nanopi-zero2.dtb.

Did not touch COMPATIBILITY.md, internal/artifacts.Version, or the
artifacts-pipeline board list beyond what gosd-rqx8 already added (kernel-only
job) - those are the flip-to-public PR's responsibility per this bean's scope
amendment.


## Summary of Changes (2026-07-07, activation PR bean/gosd-wskc-nanopi-activation)

Flipped nanopi-zero2 from `boards.RegisterInternal` to public `boards.Register`
in cmd/gosd/build.go now that gosd-f39b's U-Boot pipeline entries are
published in the real `artifacts/v0.2.0` GitHub release. Bumped
`internal/artifacts.Version` to `v0.2.0` (removed the stale
not-yet-published caveat). Updated the no---board integration test to expect
all four public images (pi-zero-2w, pi-zero-w, radxa-zero-3e,
nanopi-zero2) with qemu-virt as the sole remaining exclusion, and replaced
the old "--catalog on nanopi-only writes nothing" test with one confirming
a real catalog entry is written. internal/catalog: added nanopi-zero2 to
boardDisplayNames ("NanoPi Zero2"), left it out of boardImagerDeviceTags so
it falls back to its raw board ID as the devices tag - the same non-Pi
handling already used for radxa-zero-3e - and updated catalog_test.go/the
golden os_list.json fixture for the fourth board. Updated CLAUDE.md (moved
nanopi-zero2 out of "reserved for planned support" into the active board-ID
list), COMPATIBILITY.md (NanoPi column moved from planned/🚧 to
code-complete ✅s, kept the no-USB/rc-pin/no-WiFi-M.2/nothing-hardware-
verified footnotes, removed the now-unused board-profile footnote), and
README.md + docs/artifacts.md (both had stale "planned NanoPi"/incomplete
board-list phrasing predating this PR - pi-zero-w was also missing from
docs/artifacts.md's pipeline description; fixed both while in the
neighborhood).

### Clean-machine acceptance (real artifacts/v0.2.0 release)

With a fresh `HOME=$(mktemp -d)` (confirmed `os.UserCacheDir()` resolves
inside it: `<fakehome>/Library/Caches` on macOS) and no `--artifacts-dir`,
no `--board`:

```
go run ./cmd/gosd build ./examples/hello -o <tmpdir>
```

Succeeded in **37.1s wall** (first run, real network download + sha256
verification against the published `artifacts/v0.2.0` release), producing
all **four** public images, each **1,358,954,496 bytes** (1296 MiB /
1.266 GiB): `hello-pi-zero-2w.img`, `hello-pi-zero-w.img`,
`hello-radxa-zero-3e.img`, `hello-nanopi-zero2.img`. No `hello-qemu-virt.img`
was produced. Cache landed at
`<fakehome>/Library/Caches/gosd/artifacts/v0.2.0/<board>/` (~218MB total
across the four public boards' cached, extracted artifacts).

Re-ran into a fresh output dir with the same `HOME` (cache intact) and
`HTTPS_PROXY=http://127.0.0.1:1` (a dead proxy - any real network attempt
would hang/fail against it): succeeded in **11.6s wall**, entirely from
cache, producing byte-identical images. Confirms the "verify once, then
fully offline" cache contract in docs/artifacts.md holds against the real
release.

### pi-zero-w 32-bit ELF spot-check

Mount-free read-back (opened the real online-built `hello-pi-zero-w.img`
directly via go-diskfs, no loop-mount/root needed - same technique
build_integration_test.go's `decodeInitramfs`/`assertELF32Arm` helpers use)
confirmed both `/app` and `/init` inside `initramfs.cpio.zst` on the boot
partition are genuine `ELFCLASS32`/`EM_ARM` binaries - i.e. the real
GOARCH=arm GOARM=6 cross-compile, not a fake. Done via a throwaway
`internal/cmd/spotcheck` program during verification, removed afterward
(not part of the deliverable); the equivalent check is already codified as
`TestBuildProducesABootableImageForPiZeroWFromFakeArtifacts`'s
`assertELF32Arm` calls in cmd/gosd/build_integration_test.go.

All quality gates pass: `go test ./...`, `go vet ./...`, `gofmt -l .`
(clean), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`
(both clean).
