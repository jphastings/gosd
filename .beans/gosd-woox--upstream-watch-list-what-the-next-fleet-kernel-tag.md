---
# gosd-woox
title: 'Upstream watch list: what the next fleet kernel tag (and mainline TF-A) unlocks'
status: todo
type: task
priority: deferred
created_at: 2026-08-21T04:43:26Z
updated_at: 2026-08-21T04:43:26Z
---

Created 2026-08-21 (JP) to collapse four separately-tracked upstream-gated
beans into one checklist, because three of them fire on the **same trigger** —
the next fleet kernel tag bump — and scattering one reminder per capability
guaranteed that a bump would service some of them and silently miss the rest.
This bean supersedes and closes gosd-36yy, gosd-vo75, gosd-nplp and gosd-cjr6;
each of their unlock paths and decision rules is carried below in full, because
a lossy summary would quietly drop capability. It has **no parent** on purpose:
it spans the NanoPi Zero2 (gosd-cwjf) and Cubie A5E (gosd-h1wv) epics.

Nothing here is blocked on GoSD. Every item is blocked on somebody else
shipping something upstream, so this bean is a **watch list**, not a plan of
work: it stays `todo`/`deferred` until a trigger fires, and then it becomes one
sequenced piece of work.

## Triggers

**T1 — a numbered mainline release ships the RK3528 USB nodes.** Watch for
`v7.2.0` (or whichever numbered stable release first contains commits
`5f3ae9b12a6c` "arm64: dts: rockchip: Add USB nodes for RK3528" and
`ff660109f412` "arm64: dts: rockchip: Enable USB 2.0 ports on NanoPi Zero2",
both 2026-06-02), or a backport of either onto an active stable/LTS line —
unlikely, since new-hardware DT additions are feature additions and are
essentially never accepted into stable per the kernel's own stable rules.
Verified tag-by-tag on 2026-07-24 (gosd-36yy): absent from v6.18/v6.18.40,
v6.19.x, v7.0.x, v7.1.x; present only on `master`/`v7.2-rc4`. Items W1, W2 and
W3 all fire on T1, because all three are "what does a new
`internal/kernelspec` `fleetKernelTag` give us".

**T2 — mainline TF-A ships a `sun55i_a523` platform.** Watch
`plat/allwinner/` in TF-A releases. Independent of T1; item W4 fires on it.

`fleetKernelTag` is `v6.18.37` today, on an actively-maintained LTS line
(`releases.json`: `"moniker": "longterm"`, `"iseol": false`), so there is no
maintenance pressure to move — only capability pressure, itemised below.

## W1 — RK3528 USB gadget on the NanoPi Zero2 (was gosd-36yy)

The single most-wanted unlock: gadget + eMMC on one board is the full
`examples/usbwebsite` flow, and nanopi-zero2 is that board post-bump. Today the
board has **no USB at all**, host or gadget, because `rk3528.dtsi` has no USB
controller node in any numbered release.

Decision rule, unchanged and load-bearing: **do not pin a `-rc` tag.**
`internal/kernelspec`'s own doc comment pins "the same mainline stable LTS tag
across every Rockchip-family board", and every mainline-fleet board uses
`TagRef` against the *stable* tree. Pinning from the unstable dev branch would
be a first-of-its-kind precedent change (the Pi boards' `CommitRef` against a
vendor's long-lived branch is a different, non-comparable case) and is JP's
call, not a side effect of a bump. Also note v7.2 is a **major**-version jump
from the 6.18 line, not a point bump, so it needs full defconfig/Kconfig
survival re-verification across every mainline-fleet board — materially bigger
than "move the string".

When T1 fires:

- Re-run tag selection against the **actual released tag**, not a remembered
  one.
- Re-verify all 7 existing Rockchip DTS patches apply at that tag. They were
  dry-run against v7.2-rc4 on 2026-07-24 and 7/7 applied with zero fuzz
  (nanopi-zero2 i2c5 + spi1; rock-4se i2c + spi + usb-dwc3-peripheral;
  radxa-zero-3e i2c3 + spi3) — that is forward-prep evidence about rc4, not a
  guarantee about the final release. Note radxa-zero-3e's upstream board DTS
  has already been refactored to `#include "rk3566-radxa-zero-3.dtsi"`, so
  upstream board files do move under us.
- Promote the nanopi-zero2 fragment's currently-inert USB symbols to required,
  per its own comment block (`build/boards/nanopi-zero2/kernel/kernel-fragment.config`,
  lines 70-97). `CONFIG_USB_DWC3` was confirmed still the right symbol name at
  v7.2-rc4. **Still unchecked, explicitly:** whether
  `phy-rockchip-inno-usb2.c`'s `of_device_id` table gained an `rk3528` entry
  alongside the new node — verify before trusting `CONFIG_PHY_ROCKCHIP_INNO_USB2`.
- Add a new DTS patch forcing `dr_mode = "peripheral"` on `&usb_host0_xhci`,
  same pattern as rock-4se's `0003-usb-dwc3-peripheral.patch`. The reasoning
  differs from rock-4se's and is worth keeping: the Type-C port here is a
  genuine OTG port (SoC default `dr_mode = "otg"`, board sets
  `extcon = <&usb2phy>`), but the board *also* has a dedicated USB 2.0 Type-A
  host port on `usb_host0_ehci`/`ohci`, so dedicating the DWC3 controller to
  gadget mode loses no host capability and gives the configfs gadget stack a
  deterministic UDC at boot instead of depending on extcon/ID-pin role-switch
  timing (the class of problem `g_mass_storage` auto-binding caused on
  rock-4se, bean gosd-z9l4). Confirm against GoSD's actual gadget bring-up
  before committing to it.
- Update `fleetKernelTag` and every stale `v6.18.37` reference repo-wide;
  update `internal/kernelspec/kernelspec_test.go`'s RequiredY-style assertions.
- Ship the build/kernelspec change **without** bumping
  `internal/artifacts.Version` (tag-first, bump-second); dispatch
  `build-artifacts.yml` on the branch so the tag run is not the job's first
  execution; do not flip COMPATIBILITY.md gadget-row statuses until after a
  real artifacts release **and** hardware verification (footnote text naming
  the old tag may be updated). Never hand-edit a `kernel.config`.

## W2 — Cubie A5E second GbE and header buses (was gosd-vo75)

All out of scope at `v6.18.37` (bean gosd-jpc8's research), for three
different reasons that need three different checks at a new tag — this is
exactly the detail a summary would have flattened:

- **Second GbE (GMAC200):** the driver and DT landed upstream **after v6.18**,
  so at a new tag check for a `gmac1` node *and* the driver. If present,
  enable in-tree.
- **Header I2C:** the nodes **already exist** in `sun55i-a523.dtsi` but are
  disabled. Enabling them follows the kernel-build DTS-patch convention
  (`status = "okay"`), and was deferred only to keep the board's first pass to
  verified ground. This one is arguably doable today; it is here because it
  rides the same artifacts release as everything else on this list.
- **Header SPI:** there are **no SPI controller nodes in `sun55i-a523.dtsi` at
  all** — nothing to set `status = "okay"` on. So this is not a patch waiting
  to be written, it is a re-survey: does the new tag *have* the nodes? Only if
  it does can a `spidev` child with an accepted compatible follow.

Update COMPATIBILITY.md rows with whatever actually lands, and nothing that
doesn't.

## W3 — Cubie A5E onboard WiFi/BT (was gosd-nplp)

JP wants the fleet featureset as complete as practical (2026-08-07), so this
stays tracked rather than dropped — but it is deferred because the onboard
module's driver is expected to be non-mainline (gosd-jpc8 found no mainline
support at the fleet tag, which is why WiFi was excluded from the board epic).

**First step when picked up, and it has not been done yet: identify the actual
module and chipset on the Cubie A5E** — Radxa docs or the schematic; likely an
AIC-family or Broadcom SDIO part — and establish its driver status: mainline,
vendor out-of-tree, or none.

**Decision rule (the fleet's mainline-only policy): if the driver is not
mainline, or not clearly headed there, this stays deferred rather than adopting
a vendor driver.** That rule is the point of this item; do not quietly relax it
because a vendor tree exists. If a mainline driver *does* exist at a future
fleet tag: kernel fragment, plus firmware through the pinned-URL manifest path
(third-party blobs are never re-hosted in our releases), plus `wifiup`
integration within the WPA2-PSK/open-network scope, plus COMPATIBILITY.md rows.

## W4 — Repin Cubie A5E's TF-A to mainline (was gosd-cjr6, trigger T2)

`build/boards/cubie-a5e/manifest.json` pins BL31 from the
`jernejsk/arm-trusted-firmware` `a523` branch at commit `b5de74a685fb`
(commit-authoritative) because **mainline TF-A has no `sun55i_a523` platform at
any release tag** (verified 2026-08-06, bean gosd-jpc8). Until it does, the
fork pin is deliberate and fine: source-compiled, BSD-3-Clause, and precedented
by the Pi boards' raspberrypi/linux commit pin. This is not technical debt to
be paid down early.

When mainline TF-A ships the platform: repin to the mainline tag — the
manifest's `tfa` section, the Dockerfile fetch path, and the README wording —
then the usual artifacts release dance.

## Cross-cutting rules for whichever bump happens

- Kernel pins are **per-family and bumped family-wide, never one board alone**:
  the mainline-fleet boards (Rockchip, Allwinner, qemu-virt) share
  `fleetKernelTag`; the Pi boards share `piZeroCommitRef` against the
  *downstream* raspberrypi/linux tree and are untouched by any of this.
- Artifact releases are **tag-first, bump-second**, with the three-way
  verification (clean-machine build, offline re-run from cache, content
  spot-check that the released artifact carries the change) recorded in the
  bean that does it.
- Kernel/U-Boot Docker builds take 20-60 minutes: run them backgrounded from
  the session that owns them, never from a subagent's background task.

## Todos

- [ ] T1 fired: a numbered stable release contains the RK3528 USB nodes
- [ ] W1 — fleet tag bump + nanopi-zero2 USB gadget enablement
- [ ] W2 — re-survey cubie-a5e GMAC200 / header I2C / header SPI at the new tag
- [ ] W3 — identify the Cubie A5E WiFi/BT module and apply the mainline-only rule
- [ ] T2 fired: mainline TF-A ships `sun55i_a523`
- [ ] W4 — repin cubie-a5e BL31 to mainline TF-A
