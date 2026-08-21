---
# gosd-cwjf
title: 'Board support: NanoPi Zero2'
status: completed
type: epic
priority: low
created_at: 2026-07-05T05:34:02Z
updated_at: 2026-08-21T04:44:36Z
---

Third supported board: FriendlyElec NanoPi Zero2 — Rockchip RK3528A (4x Cortex-A53, arm64), 1/2GB LPDDR4X, GbE RJ45, microSD + eMMC socket, USB 2.0 Type-A host + USB-C device port (gadget candidate), 30-pin FPC GPIO connector (NOT a Pi-style header), 45x45mm, 5V/2A USB-C power. WiFi only via optional M.2 Key-E module — Ethernet-first support; M.2 WiFi module support is explicitly out of scope until a specific module is chosen.

Board ID (reserved in CLAUDE.md): nanopi-zero2.

Boot chain: Rockchip BootROM → idbloader (LBA 64) → u-boot.itb (LBA 16384) → extlinux — the SAME pattern as the Radxa Zero 3E, so internal/image and the pipeline need no layout changes; this epic is mostly artifacts + a board profile.

KEY RISK, verify before any build work: mainline Linux and mainline U-Boot support for RK3528/this board (FriendlyElec vendor images run a BSP 6.1 kernel, which violates our mainline-only policy). Decision rule: if mainline DT/U-Boot support is absent or immature, this epic WAITS for mainline — we do not adopt vendor BSPs.

Do not start before v0.2 ships. Hardware purchase (board + USB-C PSU + FPC breakout for GPIO testing) needed for the bring-up task.

Refs: https://wiki.friendlyelec.com/wiki/index.php/NanoPi_Zero2 , https://www.cnx-software.com/2024/09/13/nanopi-zero2-tiny-headless-arm-linux-computer-with-gigabit-ethernet-usb-port-and-m-2-key-e-socket/



## Mainline viability research (gosd-vcae): GO

Research completed 2026-07-06 — see bean gosd-vcae for full source-linked findings.
Mainline Linux has a board DT (`rk3528-nanopi-zero2.dts`, since v6.18-rc1) and mainline
U-Boot has a dedicated `nanopi-zero2-rk3528_defconfig` (landing in v2026.07). No vendor
BSP kernel needed. The KEY RISK called out above is resolved: this board can be built the
same mainline-only way as the Radxa Zero 3E. Epic is unblocked to proceed after v0.2 ships.

## USB gate discovered during kernel build (2026-07-06, PR #33)
rk3528.dtsi has NO USB controller node in any numbered kernel release as of v6.18.37 — the RK3528 dwc3 node is merged on Linus master only. Consequence: the NanoPi Zero2 has no USB at all (host or gadget) until a future fleet-wide KERNEL_TAG bump picks it up; Ethernet/SD/serial are unaffected. The bring-up task (gosd-odp7) should expect no USB, and the v0.3 gadget story excludes this board until then. Recheck when bumping KERNEL_TAG past the release containing the rk3528 dwc3 node. Follow-up researched in bean gosd-36yy (fleet kernel tag bump): as of 2026-07-24 the node (commits 5f3ae9b12a6c + ff660109f412, both 2026-06-02) is only on mainline's v7.2-rc4, not in any numbered release yet — bump deferred until v7.2.0 (or a backport) ships; gosd-36yy has the full evidence and a pre-baked plan for when it does.


## Summary of Changes

**Closed 2026-08-21 (JP).** The board is supported and shipping: mainline
viability verified (gosd-vcae), U-Boot build pipeline (gosd-f39b), trimmed
mainline kernel (gosd-rqx8), board profile with extlinux plus the raw
bootloader writes (gosd-wskc), and hardware bring-up with boot-time
measurement (gosd-odp7) are all complete. nanopi-zero2 is a public board with a
COMPATIBILITY.md bring-up row and artifact fixtures; `gosd build` builds it by
default.

The epic's last open child was gosd-36yy, the fleet kernel tag bump that would
unlock USB on this board — and it is not work this epic can do. It is blocked
on upstream: `rk3528.dtsi` gains its USB controller nodes only on mainline
`master`/`v7.2-rc4` (commits `5f3ae9b12a6c` and `ff660109f412`, both
2026-06-02) and in no numbered release. **That capability is now tracked by
gosd-woox**, the single upstream watch list, which carries the trigger, the
never-pin-an-rc decision rule, the verified DTS-patch dry-run, the fragment
symbol promotion and the `dr_mode = "peripheral"` patch plan. When gosd-woox
fires, the NanoPi Zero2 gets USB host *and* gadget — and with its eMMC socket
that makes it the full `examples/usbwebsite` board.

Closing this epic is not a claim that the board is finished. What remains, and
where it lives:

- **USB (host and gadget): still ❌ in COMPATIBILITY.md**, tracked by
  gosd-woox. The status flips only after a real artifacts release and hardware
  verification, per the locked no-flip rule.
- **gosd-0abt** — a field report that U-Boot proper found 0 bootflows on an SD
  card its own SPL had booted from — is open, parentless and unaffected by
  this closure.
- WiFi remains out of scope for this board: it has none onboard, only an
  optional M.2 Key-E slot, and no specific module has been chosen.
