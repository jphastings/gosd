---
# gosd-dkqb
title: 'pi-zero-w SPI is disabled in the shipped DTB: dtparam=spi=on is a silent no-op'
status: in-progress
type: bug
priority: high
created_at: 2026-07-29T22:02:53Z
updated_at: 2026-08-20T03:41:40Z
---

Found while researching audio (epic gosd-qkbl), unrelated to it.
COMPATIBILITY.md claims SPI works on every board, and
`internal/boards/pizerow/templates/config.txt.tmpl` carries
`dtparam=spi=on` to make it so. On pi-zero-w that line does nothing.

## Evidence

`dtc -I dtb -O dts` on the `bcm2835-rpi-zero-w.dtb` from a real
`gosd build-kernel` run (pinned raspberrypi/linux commit
`63598c83153e19b1f99067ab6df7409de2c111f8`):

- `spi: spi@7e204000 { compatible = "brcm,bcm2835-spi"; ... status = "disabled"; }`
- the DTB contains **zero** `__overrides__` nodes.

`dtparam=<x>=on` is implemented by the Pi firmware patching the DTB's
`__overrides__` block at boot. No `__overrides__`, nothing to patch — the
parameter is accepted and discarded. pi-zero-w is the one board GoSD builds
from the **mainline-style** DTS chain (`bcm2835-rpi-zero-w.dts` ->
`bcm2835.dtsi`/`bcm283x.dtsi`), and that chain has no `__overrides__` node
anywhere in it, unlike the downstream-style `bcm2710-*.dts` files
pi-zero-2w and pi-3b build (which do, including an `audio =` entry that
rewrites `chosen`'s `bootargs`). This is another instance of CLAUDE.md's
"know a Pi DTB's lineage" rule, which already records DMA and USB-gadget
versions of the same trap.

So on pi-zero-w today: `/dev/spidev0.*` never appears and
`examples/spiloopback` cannot work.

I2C is fine, but for an unrelated reason and not because of the dtparam:
`bcm2835-rpi.dtsi` sets `&i2c0`/`&i2c1` to `status = "okay"` outright, and
the built DTB confirms `i2c@7e804000 { status = "okay"; }`. So
`dtparam=i2c_arm=on` in the same template is also a no-op on this board —
it just happens not to matter.

## Fix shape

A DTS patch under `build/boards/pi-zero-w/kernel/patches/` setting
`&spi { status = "okay"; ... }` with the `spi0_gpio7` pinctrl and the two
`spidev` child nodes, in the same shape as the Rockchip boards' SPI patches
(`build/boards/rock-4se/kernel/patches/0002-enable-header-spi.patch`) — the
mainline-style DTB needs the same treatment as a no-overlay Rockchip board.
That makes it a kernel-artifact change, so it takes the tag-first,
bump-second artifacts dance in `docs/artifacts.md`.

Worth auditing the pi-zero-w `config.txt` template at the same time: any
other `dtparam=` line in it is also doing nothing, and should either become
a DTS patch or come out with a comment saying why it can't work here.

## Todo

- [x] Confirm the same conclusion for any other `dtparam=`/`dtoverlay=` line the pi-zero-w template carries (`--usb-gadget` ships `dtoverlay=dwc2` — does the firmware apply a `.dtbo` to an `__overrides__`-less DTB? Overlays and dtparams take different firmware paths, so verify rather than assume) — CONFIRMED different: bean gosd-spjt bench-proved (real Zero W hardware) that a `.dtbo` overlay applies fine to this same `__overrides__`-less mainline-style DTB ("Loaded overlay 'dwc2'" in the firmware log, UDC appeared). Overlays are FDT-fragment merges done unconditionally by start.elf; `__overrides__` is only consulted for parameterized dtparam/dtoverlay-param lookups. `dtoverlay=dwc2` is unaffected by this bug.
- [x] DTS patch enabling `&spi` + `spidev` children against the pinned commit — `build/boards/pi-zero-w/kernel/patches/0003-enable-header-spi.patch`, modelled on `rock-4se/kernel/patches/0002-enable-header-spi.patch`: `&spi { status = "okay"; pinctrl-0 = <&spi0_gpio7>; }` plus `spidev@0`/`spidev@1` (both chip selects are header-routed on this board, per its own `gpio-line-names`).
- [x] Verify with `dtc -I dtb -O dts` that the built DTB carries the enabled node — done without a full `gosd build-kernel` run (no Docker in this environment): sparse-cloned raspberrypi/linux at the exact pinned commit (`git fetch --depth 1 --filter=blob:none` + `sparse-checkout` on `arch/arm/boot/dts` and `include/dt-bindings`, the same commit-pin fetch shape as `internal/kernelbuild`'s own clone step), applied 0001->0002->0003 for real with `patch -p1 --forward`, then preprocessed+compiled `bcm2835-rpi-zero-w.dts` with `clang -E` + `dtc -I dts -O dtb`. Baseline (pre-patch) compile independently reproduced the bean's original diagnosis (`spi@7e204000` disabled, zero `__overrides__` nodes). Patched compile: `status = "okay"`, `pinctrl-0` resolves to the `spi0-gpio7` phandle (pins 7/8/9/10/11), both `spidev@0`/`spidev@1` present with `compatible = "rohm,dh2228fv"`, `__overrides__` still absent (expected - the fix hardcodes the node rather than routing through dtparam). `patch -p1 --dry-run --forward` also confirmed clean both directly against the pristine pinned source and chained after 0001+0002.
- [x] COMPATIBILITY.md: the SPI row/footnote is wrong for pi-zero-w until this lands — chose a footnote over flipping the checkmark to X, since the fix is code-complete in tree and only the (already-queued) artifacts release is pending: the top-line "Enables I2C, SPI and GPIO by default" claim now carves out pi-zero-w's SPI pending the release, and the "Pi Zero W" board note explains the mechanism, the fix location, and that /dev/spidev0.* will not appear until internal/artifacts.Version is bumped. docs/runtime.md's I2C and SPI tables/prose got the matching per-board correction (I2C: different mechanism, not affected; SPI: different mechanism, pending release).
- [ ] Bench: a real Pi Zero W with a real artifacts release (0003 patch shipped + internal/artifacts.Version bumped) shows `/dev/spidev0.0`/`/dev/spidev0.1` and `examples/spiloopback` passes with MOSI jumpered to MISO — leave unchecked, nobody has run SPI on this board yet and it cannot be verified before the release ships and the pin is bumped

## Independent verification (2026-07-30) — the mechanism is broader than SPI

Checked against the **released** `artifacts/v0.8.0` pi-zero-w tarball (downloaded fresh, not a local build), parsing the flattened device tree directly:

- `__overrides__` node: **absent** (zero occurrences in the DTB). The Pi firmware implements `dtparam=` by looking parameters up in that node, which is a *downstream* Raspberry Pi DTS convention — so on this board's mainline-style `bcm2835-rpi-zero-w.dtb`, **every `dtparam` line in config.txt is silently discarded**, not just the SPI one.
- Node states in the shipped DTB:
  - `/soc/spi@7e204000`, `/soc/spi@7e215080`, `/soc/spi@7e2150c0` → all **`disabled`**
  - `/soc/i2c@7e205000`, `/soc/i2c@7e804000`, `/soc/i2c@7e805000` → all **`okay`**

So the two features diverge, and the bean should treat them separately:
- **SPI is genuinely broken** on pi-zero-w — controllers disabled, `dtparam=spi=on` a no-op, `/dev/spidev0.*` cannot appear. COMPATIBILITY.md's ✅ is wrong and should become ❌ or 🚧 with a footnote until fixed.
- **I2C works, but by luck, and asserted by nothing** — mainline's DTS leaves the i2c nodes `okay`, so the feature is available even though its `dtparam=i2c_arm=on` is ignored. The ✅ is accidentally correct. Worth an explicit assertion (or at least a comment) so a future DTB change can't silently remove it, per the same reasoning as the defconfig-promotion traps in CLAUDE.md.

Corroborating bench evidence already in hand: the Zero W bring-up serial capture (bean gosd-qltr, session 2) printed

    dtparam: i2c_arm=on
    Unknown dtparam 'i2c_arm' - ignored

which was noted at the time as an unexplained curiosity. This is its explanation.

Fix direction (unchanged in shape, wider in scope): enable the header-routed SPI controller in the pi-zero-w DTB via the kernelspec DTS-patch mechanism — the same route bean gosd-1ey5 used for this board's `dma-ranges` fix, and the correct one here because the dtparam path structurally cannot work on a DTB without `__overrides__`. While there, decide whether to assert the i2c nodes too rather than relying on mainline's default.

**Per-board scope to check before assuming this is Zero-W-only:** pi-zero-2w and pi-3b ship *downstream*-style DTBs (`bcm2710-*`), which normally DO carry `__overrides__` — so their dtparams are probably honoured, but that must be verified the same way (parse the released DTB, don't assume) before COMPATIBILITY.md's I2C/SPI ✅ can be trusted on those boards either. The Rockchip boards are unaffected: they use kernel-build DTS patches, not dtparams.


## Summary of Changes

- **`build/boards/pi-zero-w/kernel/patches/0003-enable-header-spi.patch`** (new): flips `&spi` to `status = "okay"` with `pinctrl-0 = <&spi0_gpio7>` (GPIO7-11, matching this board's own `gpio-line-names`), and adds `spidev@0`/`spidev@1` (`compatible = "rohm,dh2228fv"`, spidev's documented generic placeholder — same convention as the Rockchip boards' SPI patches). Both chip selects are header-routed on this board (physical pins 24/26), so both get a child node, mirroring nanopi-zero2's spi1 patch rather than the single-CS radxa-zero-3e/rock-4se ones. Verified with a manual sparse-clone + `patch --dry-run`/apply + `dtc` compile against the pinned commit — see the checked Todo items above for the exact method and results.
- **`internal/boards/pizerow/templates/config.txt.tmpl`** + **`templates_test.go`**: kept both `dtparam=i2c_arm=on` and `dtparam=spi=on` (did not remove or relocate them) and added a comment block explaining they're no-ops on this board specifically, why, and what actually makes each feature work now (I2C: upstream default; SPI: the new DTS patch). See "The i2c_arm decision" below for the reasoning.
- **`COMPATIBILITY.md`**: the "Enables I2C, SPI and GPIO by default" top-line now carves out pi-zero-w's SPI pending an artifacts release, alongside the existing Cubie A5E carve-out. The "Pi Zero W" board note gained a paragraph explaining the mechanism, the fix's location, and that `/dev/spidev0.*` will not appear on a real device until `internal/artifacts.Version` is bumped to a release containing this patch.
- **`docs/runtime.md`**: the I2C and SPI per-board tables/prose got pi-zero-w-specific notes (I2C: works, but by upstream default rather than the dtparam; SPI: needs the DTS patch, pending the release) and softened the pi-zero-2w/pi-3b claims from asserted-honoured to expected-but-not-independently-reverified (matching the bean's own "per-board scope to check" caveat, so this doesn't quietly repeat the kind of over-claim the bean itself was found by).
- **Not changed**: `internal/artifacts.Version` — deliberately, per `docs/artifacts.md`'s tag-first, bump-second rule. This PR ships an `artifacts:` change file; a separate follow-up PR bumps the pin once a release containing this patch exists, and does the three-way verification (clean-machine build, offline re-run, content spot-check) at that point.

### The i2c_arm decision

`dtparam=i2c_arm=on` stays in `config.txt.tmpl`, unmodified in behavior (I2C's on-device state does not change either way — it's on regardless, via `bcm2835-rpi.dtsi`'s unconditional `status = "okay"`), with a new comment explaining why the line itself does nothing here. Considered three options:
1. **Leave silently** — rejected: reproduces exactly the confusion bean gosd-qltr recorded as an "unexplained curiosity" (`Unknown dtparam 'i2c_arm' - ignored` on the serial console) with nothing in the source pointing a future reader at the explanation.
2. **Remove the line** — rejected: it's cross-board "locked content" (beans gosd-06kj/gosd-85pt/gosd-fnza expect every Pi board's `config.txt` to carry it, matching pi-zero-2w/pi-3b), and removing it would look like a claim that I2C support changed, when nothing about I2C's actual behavior does.
3. **Comment it, keep the line** (chosen): fully preserves I2C's on-device behavior (satisfies "do not silently change I2C behaviour" exactly), keeps the cross-board file shape consistent, and stops the no-op from being silently rediscovered as a mystery a second time. `dtparam=spi=on` got the same comment for the same reason, now that SPI is *also* achieved by a different mechanism (the DTS patch) rather than by this line.
