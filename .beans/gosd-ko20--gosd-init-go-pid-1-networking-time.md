---
# gosd-ko20
title: 'gosd-init: Go PID 1, networking & time'
status: completed
type: epic
priority: normal
created_at: 2026-07-02T20:49:56Z
updated_at: 2026-08-21T01:41:56Z
parent: gosd-sc9w
---

A single static Go binary that is PID 1 inside the initramfs. It owns: early mounts, launching/supervising the user app, network bring-up (Ethernet DHCP + WiFi WPA2-PSK), and clock sync. No shell, no busybox, no systemd — if gosd-init can't do it in Go, it doesn't happen.

Locked library choices: netlink via github.com/vishvananda/netlink; DHCPv4 client via github.com/insomniacslk/dhcp/dhclient4; WiFi via nl80211 using github.com/mdlayher/wifi (brcmfmac has firmware SME, so WPA2-PSK connect via nl80211 CONNECT command works — same approach gokrazy uses); SNTP via github.com/beevik/ntp.

gosd-init lives in this repo (cmd/gosd-init) and is cross-compiled and embedded by the CLI at build time.

## Summary of Changes

`cmd/gosd-init` is a single static Go binary that is PID 1: no shell, no
busybox, no systemd. gosd-kkz4 built the core — early mounts, the boot
sequence, `/app` launch and supervision with backoff, SIGCHLD reaping, and
console logging. gosd-vtce added wired networking (link up via netlink,
DHCPv4, DNS) as `netup`, deliberately asynchronous so it can never block app
start — which is why there is no `GOSD_IP` env var and an app discovers its
own address. gosd-fbwa added `wifiup`: WPA2-PSK association over raw
nl80211, with gosd-lbpm extending it to hidden SSIDs. gosd-c8oj added
`timesync`, SNTP with a sanity floor and a maximum step, publishing
`/run/gosd/time-synced` for apps that need a trustworthy clock before making
TLS calls. All four run under `StartNetworking`'s panic guard in
`main.go`.

The shape these set is now the house pattern for every gosd-init feature
(mDNS, ingress agents, the config store): pure logic behind a small
interface seam with fake-driven tests that pass on macOS, real syscalls
isolated in `platform_linux.go` with `platform_other.go` stubs.

The costliest lesson lives in CLAUDE.md rather than here: raw netlink
through `mdlayher/netlink` must OR `netlink.Request` into its flags, or the
kernel silently skips the message while still acking it — two bench days
(gosd-anyp), now pinned by `wifiup/connect_linux_test.go`.

Hardening found later (reaper races, DHCP renewal loops, resolv.conf
atomicity, address flushing on link-down, panic recovery) landed as its own
bug beans against the same packages, not as reopened scope here.
