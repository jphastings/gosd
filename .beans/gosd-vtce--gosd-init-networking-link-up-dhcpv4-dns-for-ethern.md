---
# gosd-vtce
title: 'gosd-init networking: link up, DHCPv4, DNS, for Ethernet'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-03T17:59:38Z
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

- [x] Implementation + retry/backoff unit tests (interface logic behind an interface so it tests without netlink)
- [x] Handles link down/up (cable re-plug) by re-running DHCP

## Acceptance
Radxa bring-up task observes:
- [ ] boot with cable → lease within 3s of link up (needs hardware; not verified in this change)
- [ ] boot without cable → app runs anyway, plugging cable later gets a lease (needs hardware; not verified in this change)

## Summary of Changes

Added `cmd/gosd-init/internal/netup`, a new package bringing up wired networking asynchronously after `/app` starts, following the `boot` package's fake-driven, thin-interface style:

- `netlink` (`Links`) and DHCPv4 (`DHCPClient`) sit behind interfaces defined in `interfaces.go`; the retry/backoff/renewal/link-flap state machine (`netup.go`, `dhcp.go`, `backoff.go`) has no build tags and is unit-tested with fakes on macOS. Real netlink/`nclient4`-backed implementations live in `platform_linux.go` (`linux` build tag); `platform_other.go` stubs them for non-Linux builds.
- `RunDHCP(ctx, deps, iface, onLease)` is exported specifically so the future WiFi bean can call it directly once its nl80211 association brings the wifi interface's carrier up — DHCP itself doesn't care about the underlying medium.
- `boot.Deps` gained one new optional field, `StartNetworking func(log func(format string, args ...any))`, called in its own goroutine right before `/app` supervision begins in `boot.Run` — networking never blocks or delays app start. This is the only change to the existing `boot` package (plus a new test for it).
- `main.go` wires `netup.NewPlatform()` and `netup.Run` into `boot.Deps.StartNetworking`.
- Link-down triggers cancelling that interface's DHCP loop and clearing `/run/gosd/network-up`; link-up (including replug) restarts DHCP from scratch. This marker-clearing on link-down is not explicitly asked for by the bean (only marker-creation is) but was added since a stale "network up" marker after an unplug would be actively misleading.
- Lease maintenance: renew at T1; on renew failure, wait until T2 and retry once (rebind); if that also fails, restart full Discover/Request. Discovery itself retries forever with jittered ("full jitter") exponential backoff, per the bean.

### Design choice: /etc/resolv.conf writability

Chose **not** to mount a dedicated tmpfs over `/etc`. gosd-init's rootfs *is* the initramfs itself — the locked 8-step boot sequence (`gosd-kkz4`) never `pivot_root`/`switch_root`s to a different filesystem — and Linux's initramfs rootfs ("rootfs") is already a RAM-backed, fully writable filesystem, not a read-only cpio image. So `/etc` is writable with no extra mount. `netup.WriteResolvConf` simply calls `os.WriteFile` with `O_CREATE`, which lazily creates `/etc/resolv.conf` on first write if the initramfs build didn't already include a placeholder. Documented in `cmd/gosd-init/internal/netup/resolvconf.go`. If a future board profile makes the rootfs read-only, that's the function to revisit.

### Deviations / follow-ups

- Added a note to `gosd-mr2n` (Go developer quickstart + runtime documentation) about `GOSD_IP` not being settable post-launch, per this bean's instruction to document that there.
- Hardware acceptance items (lease-within-3s-of-link-up timing, cable-plugged-in-later behavior) require the Radxa and are left unchecked above; this PR only covers the code and its unit tests. Kept the bean `in-progress` rather than `completed` for that reason, even though both code todos are checked.
