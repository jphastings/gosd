---
# gosd-ko20
title: 'gosd-init: Go PID 1, networking & time'
status: todo
type: epic
created_at: 2026-07-02T20:49:56Z
updated_at: 2026-07-02T20:49:56Z
parent: gosd-sc9w
---

A single static Go binary that is PID 1 inside the initramfs. It owns: early mounts, launching/supervising the user app, network bring-up (Ethernet DHCP + WiFi WPA2-PSK), and clock sync. No shell, no busybox, no systemd — if gosd-init can't do it in Go, it doesn't happen.

Locked library choices: netlink via github.com/vishvananda/netlink; DHCPv4 client via github.com/insomniacslk/dhcp/dhclient4; WiFi via nl80211 using github.com/mdlayher/wifi (brcmfmac has firmware SME, so WPA2-PSK connect via nl80211 CONNECT command works — same approach gokrazy uses); SNTP via github.com/beevik/ntp.

gosd-init lives in this repo (cmd/gosd-init) and is cross-compiled and embedded by the CLI at build time.
