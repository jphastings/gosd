---
# gosd-fnza
title: 'SPI: /dev/spidev on every board, loopback example, pin docs'
status: in-progress
type: feature
priority: normal
created_at: 2026-07-08T03:35:00Z
updated_at: 2026-07-09T20:05:43Z
parent: gosd-jge2
blocked_by:
    - gosd-nyad
---

Expose and document SPI on all four boards. Mirrors the I2C work (bean gosd-85pt). Branch this from the GPIO branch, not main, so the shared COMPATIBILITY/runtime edits stack cleanly.

DRIVER STATE (verify, do not assume): Pis have SPI_BCM2835=y + SPI_SPIDEV=y already. Rockchip fragments have SPI=y + SPI_ROCKCHIP=y but SPI_SPIDEV is NOT visibly in the fragment — CHECK the committed generated kernel.config for radxa + nanopi; if SPI_SPIDEV isn’t =y there, add CONFIG_SPI_SPIDEV=y to those fragments (forces a kernel rebuild).

LOCKED DECISIONS / per-board mechanism:
- Pis: add `dtparam=spi=on` to both config.txt templates (+ render tests) -> /dev/spidev0.0 and 0.1 on the standard header pins (19 MOSI/21 MISO/23 SCLK/24 CE0/26 CE1). No artifact release for the Pi half.
- Rockchips (radxa rk3566, nanopi rk3528): read the DTS at v6.18.37 — which SPI controller routes to the 40-pin header / 30-pin FPC, its status, and whether a `spidev` child node (compatible per current kernel policy — check what the kernel accepts; some versions warn on bare "spidev") + status="okay" is needed. Enable via a kernel-build DTS patch (SAME mechanism as I2C: our pinned U-Boots lack OF_LIBFDT_OVERLAY — reuse that established finding). Record bus + pins.
- ARTIFACT RELEASE: any Rockchip fragment/DTS change means new artifacts (v0.4.0) before real non---artifacts-dir builds see it. State prominently in the PR; the tag-then-bump follow-up (like gosd-xshg did for v0.3.0) is a SEPARATE task after merge — note it, do not do it in this PR.
- Example: examples/spiloopback — raw SPI_IOC_MESSAGE ioctl via golang.org/x/sys/unix (no new dep, consistent with i2cscan), does a MOSI->MISO loopback transfer (documented "jumper MOSI to MISO"), reports match/mismatch, graceful when no spidev present. periph.io as the pointer for real apps.

Per-board work:
- [x] Verify/add SPI_SPIDEV=y in Rockchip fragments; Pi config.txt dtparam=spi=on + render tests
- [x] Rockchip DTS patches (bus enable + spidev node) with per-board bus/pin findings recorded
- [x] examples/spiloopback (x/sys ioctl, both arches, CI)
- [x] docs/runtime.md SPI section (per-board bus/pins) + COMPATIBILITY SPI row -> code-complete
- [x] Note the required v0.4.0 artifact release + follow-up bump task
- [ ] Bench: loopback passes on each board (leave unchecked)

## Acceptance
Fake-artifact integration tests show config.txt dtparam (Pis) and the DTS/fragment changes present; example compiles both arches; docs correct. Real loopback + the artifact re-release remain follow-ups.

## Summary of Changes

SPI is now enabled by default on all four boards, code-complete and
fake-artifact-tested (bench verification with a real MOSI-to-MISO jumper
remains the one unchecked item, as scoped).

- **Pi Zero 2W / Zero W**: `dtparam=spi=on` added to both `config.txt`
  templates (unconditional, no opt-out) -> `/dev/spidev0.0` and
  `/dev/spidev0.1` on the standard header pins (19 MOSI/21 MISO/23 SCLK/24
  CE0/26 CE1).
- **Radxa Zero 3E**: `spi3` (pinmux `spi3m1_pins`/`spi3m1_cs0`, GPIO4_C2/C3/
  C5/C6) routes to the 40-pin header's pins 19 (MOSI)/21 (MISO)/23 (SCLK)/24
  (CS0), confirmed against Radxa's own schematic/pinout docs
  (`docs.radxa.com/en/zero/zero3/hardware-design/hardware-interface`) —
  mainline enables `spi3` with its default "m0" pin group (disabled, no
  board owner); the header wiring uses "m1" instead, so the patch overrides
  `pinctrl-0` rather than only flipping `status`. `rk356x-base.dtsi` already
  aliases every `spiN`, so it's guaranteed to enumerate as bus 3. Only CS0 is
  header-routed — physical pin 26, where a Pi's CE1 would be, is **NC** on
  this board's header (confirmed against the same schematic), so
  `spi3m1_cs1` is intentionally omitted and there is no `/dev/spidev3.1`.
- **NanoPi Zero2**: `spi1` (GPIO1_B6/B7/C0/C1/C2) routes to the 30-pin FPC's
  pins 16 (CLK)/17 (MOSI)/18 (MISO)/19 (CS0)/20 (CS1), read from
  FriendlyElec's own schematic (`NanoPi_Zero2_2407_SCH.pdf`'s GPIO table/
  pinout diagram) — both chip selects are routed to the connector, so both
  `spidev@0` and `spidev@1` are added. Pin numbers were anchored against the
  already-established, bench-adjacent-verified I2C5 pins (12/13 = GPIO1_B2/
  B3, from bean `gosd-85pt`) and counted consecutively through the GPIO1-bank
  cluster on the same schematic diagram, since the diagram's own physical
  pin numbering isn't printed as a single unambiguous column — flagging this
  reasoning explicitly rather than presenting it as read directly off a
  labeled pin. `rk3528.dtsi` doesn't pre-alias any `spiN` (this board's own
  `.dts` already has to alias `i2c5` explicitly for the same reason), so the
  patch adds a `spi1 = &spi1;` alias too, to guarantee `/dev/spidev1.*`.
- **Mechanism for both Rockchip boards**: a kernel-build-time DTS patch
  (`build/boards/{radxa-zero-3e,nanopi-zero2}/kernel/patches/0002-enable-
  header-spi{3,1}.patch`), same mechanism as the I2C patches (pinned U-Boots
  lack `CONFIG_OF_LIBFDT_OVERLAY`). **Verified for real**, not just
  `git apply --check`: ran a Docker dry-run (debian:bookworm, no full kernel
  build) that clones the pinned `v6.18.37` tag, applies both boards' patches
  in sequence (0001-i2c then 0002-spi), runs `make defconfig` +
  `merge_config.sh` + `make olddefconfig`, and actually compiles the real
  board `.dtb` (not just the Image) — both boards' DTBs built cleanly with
  no new warnings. Decompiled both resulting `.dtb`s with `dtc -I dtb -O
  dts` and confirmed `spi@fe640000` (radxa, alias `spi3`, `status=okay`,
  `spidev@0` with `compatible=rohm,dh2228fv`) and `spi@ff9d0000` (nanopi,
  alias `spi1`, `status=okay`, two `spidev@N` children) are present in the
  compiled output. **This needs a new artifacts release** (DTB rebuild) —
  `artifacts/v0.4.0` — before a real, non-`--artifacts-dir` `gosd build`
  picks up the change on either Rockchip board, same tag-then-bump dance as
  `v0.3.0` (bean `gosd-85pt-artifacts-v030`). That release, and the
  follow-up `internal/artifacts.Version` bump once it's tagged, are
  separate follow-up work — not done in this PR.
- **`CONFIG_SPI_SPIDEV`**: checked the committed generated `kernel.config`
  for both Rockchip boards first, per the bean's instruction — it was
  already `=y` on both, but only via the arm64 `defconfig` baseline (same
  "lands `=y` without being asked" shape as the NanoPi Zero2 kernel README's
  USB-stack diff-table note), not because either fragment named it
  explicitly. Added `CONFIG_SPI_SPIDEV=y` to both fragments explicitly, and
  to the `required_y` assertion list in both `docker-build.sh`s, so a future
  kernel bump silently dropping it (the way a `defconfig` baseline easily
  could) now fails the build loudly instead of quietly losing every
  `/dev/spidev*`.
- **`spidev` DT `compatible`**: checked `drivers/spi/spidev.c` and
  `Documentation/spi/spidev.rst` directly at the pinned tag — a bare
  `compatible = "spidev"` node is explicitly refused
  (`spidev_of_check()` logs "spidev listed directly in DT is not supported"
  and returns `-EINVAL`). Used `"rohm,dh2228fv"`, one of the generic
  placeholder compatibles spidev's own `spidev_dt_ids[]` table lists for
  exactly this purpose — the same one Raspberry Pi's own downstream spidev
  overlays use — and documented why in both the DTS patch comments and
  `docs/runtime.md`.
- `examples/spiloopback`: opens every `/dev/spidev*`, and performs a
  full-duplex `SPI_IOC_MESSAGE(1)` ioctl transfer of a fixed test pattern,
  reporting a match (loopback confirmed) or mismatch (expected without the
  documented MOSI-to-MISO jumper) — `golang.org/x/sys/unix` only, no new
  module dependency, computing the `SPI_IOC_MESSAGE(N)` request number the
  same way the kernel's own macro does rather than hardcoding a
  single-message magic constant. Exits 0 with a clear message when no
  `/dev/spidev*` exists (e.g. under `qemu-virt`, which has no SPI
  controller). Unlike `examples/i2cscan`'s `i2c_msg` struct (whose buffer
  fields are natively pointer-typed, so the GC already tracks them), the
  real kernel `spi_ioc_transfer` struct's `tx_buf`/`rx_buf` are fixed-width
  `__u64` regardless of architecture (so the 32-bit `pi-zero-w` build stays
  ABI-correct), which means the buffer pointers have to pass through
  `uintptr` — `runtime.KeepAlive` on both buffers after the syscall keeps
  them reachable until the kernel has actually dereferenced those addresses.
  Builds clean under `go vet`/`golangci-lint` on both `arm64` and
  `arm`/`GOARM=6`, added to the CI smoke-build job's cross-compile lists.
- `docs/runtime.md`: added an "SPI is on by default" section (per-board
  device/pin table, the `spidev` compatible note, a pointer at
  `examples/spiloopback`/periph.io) and updated the "GPIO, I2C, SPI" intro's
  now-stale "SPI's is tracked for a future release" line.
- `COMPATIBILITY.md`: flipped the SPI row to ✅ on all four boards and
  rewrote the `[^spi]` footnote to match the I2C footnote's shape — code
  mechanism, the v0.4.0 artifact-release caveat, the Radxa single-chip-select
  limitation, and pointers at the docs/example.
- Fake-artifact integration test: added a `dtparam=spi=on` assertion to both
  `TestBuildProducesABootableImageFromFakeArtifacts` (pi-zero-2w) and
  `TestBuildProducesABootableImageForPiZeroWFromFakeArtifacts` (pi-zero-w)
  in `cmd/gosd/build_integration_test.go`.

Deviation from the bean's literal wording: the NanoPi Zero2 FPC's physical
pin numbers for the SPI1 signals (16/17/18/19/20) are derived by anchoring
to the already-verified I2C5 pin numbers on the same schematic diagram and
counting consecutively through the same GPIO1-bank cluster, rather than
read directly off an unambiguous per-pin label the way the Radxa's SPI
pins were (docs.radxa.com's rendered pinout table has one column per
physical pin number; FriendlyElec's schematic diagram doesn't). Flagging
this so the bench-verification step double-checks these five pins
specifically against a multimeter/continuity check before trusting them
for real wiring.



## v0.4.0 artifact release — live and pinned (bean gosd-fnza-v040)

The tag-first follow-up is done: `artifacts/v0.4.0` is published (all five board tarballs) and `internal/artifacts.Version` is bumped v0.3.0 -> v0.4.0 on branch `bean/gosd-fnza-artifacts-v040`. The SPI half of this bean is now live on real (non-`--artifacts-dir`) builds for both Rockchip boards. Bench loopback (the one unchecked todo) remains the only open item; this bean stays in-progress.

Real-release clean-machine + SPI-DTB verification (recorded in tracking bean gosd-fnza-v040):
- Clean-machine build (fresh HOME, no --board/--artifacts-dir): downloaded + sha256-verified artifacts/v0.4.0, produced FOUR public images (~1.27 GiB each), ~2m32s.
- Offline re-run (dead HTTPS_PROXY, fresh output dir, same HOME): succeeded fully from cache in ~18s.
- SPI-DTB spot-check on the DTBs the build actually pulled: radxa spi3 (spi@fe640000, aliased spi3) status="okay" with spidev@0 compatible="rohm,dh2228fv"; nanopi spi1 (spi@ff9d0000, aliased spi1) status="okay" with spidev@0 and spidev@1 both compatible="rohm,dh2228fv". Pi config.txt carries dtparam=spi=on.
