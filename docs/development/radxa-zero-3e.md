# Developing for the Radxa Zero 3E (`radxa-zero-3e`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-v370`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md; this file is for things a future agent or developer would
otherwise have to rediscover by hand. The CP210x/PL2303 1,500,000-baud
console-garbling workaround is already documented in COMPATIBILITY.md's
board notes and [the serial console baud rate flag](../runtime.md) — it is
not repeated here.

## Bring-up is not finished

`gosd-nlzf` (hardware bring-up and boot-time measurement) is still
**in-progress**, and the epic (`gosd-v370`) is still open because of it. As
of the last recorded session (2026-07-24), the board boots the full chain
end to end on two independent units — TPL/SPL → U-Boot → kernel →
gosd-init → app — takes a DHCP lease and answers mDNS queries. Still
outstanding: the boot-time baseline (power-to-U-Boot, U-Boot handoff,
kernel-to-init, power-to-HTTP), the 5x power-cycle survival run, and the
USB-gadget/GbE/I2C/SPI/GPIO peripheral checks. Part of what's blocking the
timing work is serial visibility during the U-Boot phase — see below.

## The serial bug is RK3566-family-specific, not a generic CP210x limit

COMPATIBILITY.md already documents the CH340/`--console-baud` workaround.
What it doesn't say (bean `gosd-zp9s`): the same CP2102N adapter, wires, and
capture setup read **rock-4se (RK3399)** and **nanopi-zero2 (RK3528)**
perfectly at 1,500,000 baud. Only this board's RK3566 UART2 TX garbles —
received bytes skew high-bits-set/low-bits-clear, consistent with slow
(RC-style) rising edges as seen by the adapter's input, not a generic
"CP210x can't do 1.5M" limitation across the Rockchip fleet. Reproduced on
two separate units and two OSes (GoSD and Armbian), ruling out unit damage.

A real fix — raising the UART2 TX pin's drive strength via a DTS patch — was
deliberately deferred to a bench session with a scope or a known-good
capture, not implemented. If you pick it up:

- It goes in `build/boards/radxa-zero-3e/kernel/patches/`, the same
  DTS-patch mechanism already used for this board's I2C/SPI enablement.
- Confirm which UART2 IOMUX/pinctrl group is actually pinned in the
  existing DTS before patching (`uart2m0-xfer` or another mux group,
  depending on the board's own pin selection) — don't assume.
- The RK3566 TRM's `drive-strength` enum range and its actual per-domain
  numbering needs checking against the TRM/vendor overlay examples, not
  assumed from common Rockchip conventions. rock-4se's and nanopi-zero2's
  drive-strength values are **not** transferable — different SoC, different
  pinctrl IP — they're only evidence that higher drive strength on some
  Rockchip part reads cleanly on this adapter at 1.5M, not a value to copy.
- `--console-baud` only rewrites `extlinux.conf`'s kernel-onward
  `console=` parameter. U-Boot's own serial output is compiled in at
  1,500,000 baud regardless of this flag, so U-Boot-phase visibility on a
  CP210x adapter needs a CH340 (or the drive-strength fix) either way.

**Useful diagnostic trick, not just a fact about this bug:** even fully
garbled output can be read for its *cadence*. The bring-up session
identified gosd-init's restart backoff (1s→2s→4s→8s→10s cap) from garbled
byte bursts alone, before a single byte decoded correctly — proof the
software stack is running end-to-end that doesn't require readable serial
at all. Worth trying on any future "adapter reads garbage" bench problem
before assuming the board is dead.

## Both bench units lack onboard eMMC

eMMC is a build-to-order option on this board, and JP's two bench units
both lack it, so the `emmc` package's `ErrNoEMMC` path gets exercised on
every bring-up session here. An earlier example message told a Zero 3E
owner to "get a Radxa Zero 3E" for eMMC — misleading, since having the
right board model isn't the same as having eMMC fitted (bean `gosd-cgpr`
reworded it to name the requirement instead of a board). Don't assume board
identity implies eMMC presence when working on this board's eMMC paths.

## Kernel build findings (bean `gosd-c7tk`)

- **Ethernet PHY is autodetected, not named in the DT.** The `&mdio1`
  node's PHY child uses the generic `compatible = "ethernet-phy-ieee802.3-c22"`
  — the actual chip is Radxa's documented RTL8211F-CG, matched at runtime
  via MDIO ID registers by `CONFIG_REALTEK_PHY`. Don't expect to grep the
  DTS for "rtl8211f" — it isn't there.
- **The bean's own Kconfig symbol name was wrong.** It names
  `CONFIG_PHY_ROCKCHIP_NANENG_COMBPHY`; the real mainline symbol (checked
  against `drivers/phy/rockchip/Kconfig` at the pinned tag) is
  `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY` — COMBO_PHY, not COMBPHY.
- **`ARCH=arm64` dtb make targets are relative to `arch/arm64/boot/dts/`.**
  Passing the full path (`arch/arm64/boot/dts/rockchip/rk3566-radxa-zero-3e.dtb`)
  as the make target doubles the prefix and fails with "No rule to make
  target" — the make target must be `rockchip/rk3566-radxa-zero-3e.dtb`,
  even though the `cp` source path afterward does need the full path.
- Pinned source at completion: mainline stable "longterm" tag `v6.18.37`.

## U-Boot build findings (bean `gosd-d458`)

- **One defconfig covers both the Zero 3E and Zero 3W.**
  `radxa-zero-3-rk3566_defconfig` builds both DTBs into the FIT
  (`CONFIG_OF_LIST` lists both `rockchip/rk3566-radxa-zero-3w` and
  `rockchip/rk3566-radxa-zero-3e`); the board auto-detects which one it is
  at runtime by reading SARADC channel 1 against a hardware-ID resistor
  (`board/radxa/zero3-rk3566/zero3-rk3566.c`: 230–270 → 3W, 400–450 → 3E)
  and `board_fit_config_name_match()` picks the DTB accordingly. One build,
  one binary, no board-specific U-Boot compile needed for this pair.
- RK3566 needs two rkbin blobs (DDR-init TPL and BL31) — and specifically
  the **rk3568** BL31/TPL blobs, not an rk3566-named one; rkbin doesn't
  ship separate rk3566 blobs.
- The Dockerfile needed `python3-dev` and `libgnutls28-dev` beyond what
  U-Boot's own doc implies, to satisfy host-tool build deps (pylibfdt,
  mkeficapsule).
- Pinned mainline U-Boot tag at completion: `v2026.04` (latest stable;
  `v2026.07` was rc-only at the time).

## Peripheral DTS patches land per artifacts release

I2C (`i2c3`) was enabled by a DTS patch shipped in `artifacts/v0.3.0` (bean
`gosd-xshg`); SPI (`spi3` + a `spidev` child) followed in `artifacts/v0.4.0`
(bean `gosd-jphp`). Both were verified against the actual published
release's DTB via `dtc -I dtb -O dts` — not just that the tarball
downloaded — so if I2C/SPI don't appear to work on this board, check which
`internal/artifacts.Version` is actually pinned before suspecting the DTS
patch itself.

## Board profile (bean `gosd-gbsz`)

`RawWrites` offsets are locked: `idbloader.img` at byte offset 32768
(LBA 64), `u-boot.itb` at byte offset 8388608 (LBA 16384) — both in the
unpartitioned gap before the 16MiB boot-partition start, with a size guard
that fails loudly if `u-boot.itb` would run into the partition. `Board.RawWrites`
has no error return, so a violation panics with an actionable message.
`FirmwareFiles()` is intentionally an empty map — no runtime-loaded firmware
is needed on this board in v0.1.

## Open question: does U-Boot waste time on a preboot USB scan here?

Bean `gosd-uj4l` found and fixed an unconditional `usb start` preboot scan
costing ~4.5s on cubie-a5e (an Allwinner-specific Kconfig `select` chain).
Bean `gosd-ylkv` splits out the same question for the Rockchip boards,
including this one — unmeasured as of this writing. The cubie-a5e root
cause (`ARCH_SUNXI` forcing `CONFIG_USB_KEYBOARD` into `PREBOOT`) doesn't
transfer to `ARCH_ROCKCHIP`; this board's own `CONFIG_PREBOOT` resolution at
the pinned U-Boot tag needs establishing from scratch before assuming there
is (or isn't) anything to fix.
