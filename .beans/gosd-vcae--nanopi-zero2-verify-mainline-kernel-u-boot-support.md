---
# gosd-vcae
title: 'NanoPi Zero2: verify mainline kernel + U-Boot support for RK3528'
status: todo
type: task
priority: low
created_at: 2026-07-05T05:34:02Z
updated_at: 2026-07-05T05:34:02Z
parent: gosd-cwjf
---

Research task, gates the whole epic. Answer with source links (kernel.org / U-Boot git, not vendor wikis):
- [ ] Does a mainline DTS exist for NanoPi Zero2 (rk3528-nanopi-zero2.dts or similar)? Since which kernel tag? If board DT is absent but rk3528.dtsi exists, list what a board DT would need (upstream it or wait — state recommendation per the mainline-only policy)
- [ ] Mainline U-Boot: is there an RK3528 board family defconfig usable for this board? Which rkbin blobs (DDR init, BL31) does RK3528 need — exact paths + license check
- [ ] GbE: which MAC/PHY (stmmac + which PHY driver)? USB gadget: which controller (dwc2/dwc3) on the USB-C device port, and is it usable as a peripheral in mainline?
- [ ] Debug UART: which UART + baud (FriendlyElec convention is usually 1500000n8 — confirm), and where the pins are on this board
- [ ] Deliver: findings appended here + a go/no-go recommendation; if no-go, set this epic priority to deferred with a "recheck at kernel vX.Y" note

## Acceptance
Every claim source-linked; a clear go/no-go on starting the build tasks.
