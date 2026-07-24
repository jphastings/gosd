---
# gosd-cwjf
title: 'Board support: NanoPi Zero2'
status: todo
type: epic
priority: low
created_at: 2026-07-05T05:34:02Z
updated_at: 2026-07-24T16:17:28Z
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
