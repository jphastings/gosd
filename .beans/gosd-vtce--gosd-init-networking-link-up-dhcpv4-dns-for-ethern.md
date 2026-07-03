---
# gosd-vtce
title: 'gosd-init networking: link up, DHCPv4, DNS, for Ethernet'
status: todo
type: task
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-02T21:03:54Z
parent: gosd-ko20
blocked_by:
    - gosd-kkz4
---

Bring up wired networking asynchronously after /app is launched.

Locked libraries: github.com/vishvananda/netlink for link handling; github.com/insomniacslk/dhcp (nclient4) for DHCPv4.

Behavior:
- Bring `lo` up. Watch for a wired interface (name match `eth*`/`end*`/`enp*`) appearing via netlink, set it up
- DHCPv4 loop: discover with retries + jittered backoff forever (cable may be plugged in late); on lease: assign addr, default route, write /etc/resolv.conf from lease DNS (create the file in the initramfs tmpfs — ensure /etc is writable: mount a tmpfs over /etc or pre-create resolv.conf writable in the initramfs; pick one, note the choice in code)
- Renewal handling per lease T1/T2
- Expose state: write /run/gosd/network-up (empty file) when an address is assigned; log addr + gateway
- Set GOSD_IP is NOT possible post-launch — the app discovers its own addr via net.Interfaces; document this in the runtime docs task

- [ ] Implementation + retry/backoff unit tests (interface logic behind an interface so it tests without netlink)
- [ ] Handles link down/up (cable re-plug) by re-running DHCP

## Acceptance
Radxa bring-up task observes: boot with cable → lease within 3s of link up; boot without cable → app runs anyway, plugging cable later gets a lease.
