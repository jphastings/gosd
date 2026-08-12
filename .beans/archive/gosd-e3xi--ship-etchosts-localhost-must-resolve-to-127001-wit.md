---
# gosd-e3xi
title: 'Ship /etc/hosts: localhost must resolve to 127.0.0.1 without touching DNS'
status: completed
type: bug
priority: high
created_at: 2026-08-07T10:02:59Z
updated_at: 2026-08-07T10:16:40Z
---

JP request (2026-08-03). GoSD images ship no /etc/hosts, and with
CGO_ENABLED=0 Go's pure resolver reads /etc/hosts directly — so an app
dialing "localhost:8080" sends the lookup to DNS: it fails before
networking is up, and after that leaks "localhost" queries to whatever
resolver the LAN handed us (which may even answer). Static externals
(musl) read /etc/hosts too.

Two parts:

1. **Bake the static entries into the initramfs** (internal/initramfs):
   `127.0.0.1 localhost` plus the conventional IPv6 lines
   (`::1 localhost ip6-localhost ip6-loopback`). Zero runtime code;
   covers every process from PID 1 onward.
2. **gosd-init appends the device-hostname line** once the hostname
   settles (after the gosd.toml/cloud-init override in boot.Run):
   `127.0.1.1 <hostname>` (Debian convention), so apps resolving
   os.Hostname() work without the network. Rewrite via the same atomic
   temp+rename pattern as resolv.conf (tmpfs, no fsync needed —
   resolvconf.go documents why); hostname is dynamic so this line can't
   be baked at build time. If the hostname is later invalid/unchanged,
   the static localhost lines must survive untouched.

Check whether /etc/nsswitch.conf matters: Go's resolver defaults to
hosts-file-first when it's absent — confirm and note in a comment
rather than shipping one.

Tests: initramfs content test asserting the baked file; boot behavioral
test that the hostname line lands (and updates when gosd.toml overrides
the hostname); resolver-level proof if cheap (net.DefaultResolver with a
test hosts path is awkward — a file-content assertion is acceptable).
Consider asserting in the qemu CI job that the running image resolves
localhost (e.g. hello logging a self-dial) only if it drops in cleanly.

Docs: runtime.md's networking section gains a line ("localhost resolves
via the shipped /etc/hosts; your device's own hostname resolves to
127.0.1.1").


## Todos

- [x] internal/hostsfile: shared static /etc/hosts content (Static/Render/Write), used by both the build pipeline and gosd-init
- [x] internal/pipeline: bake /etc/hosts' static lines into the initramfs; add to the ComputeIdentity payload (and its docstring)
- [x] cmd/gosd-init/internal/boot: Deps.WriteHosts, called once cfg.Hostname has fully settled (after gosd.toml/cloud-init AND any provisioning-snapshot restore), never fatal on failure
- [x] cmd/gosd-init/main.go: wire WriteHosts to hostsfile.Write
- [x] Confirm Go's resolver is hosts-file-first without /etc/nsswitch.conf (net/conf.go's hostLookupFilesDNS fallback) and record it in hostsfile's package doc instead of shipping the file
- [x] Tests: hostsfile unit tests, a pipeline test asserting the baked file, boot.Run tests (single write with the final hostname, failure is non-fatal, provisioning-snapshot-restored hostname reaches /etc/hosts)
- [x] docs/runtime.md networking section gains a line
- [x] Quality gates + PR

## Summary of Changes

- Added `internal/hostsfile`, a small dependency-free package shared by the build side (`internal/pipeline`) and gosd-init (`cmd/gosd-init/internal/boot`): `Static()` returns the baked `127.0.0.1 localhost` / `::1 localhost ip6-localhost ip6-loopback` lines, `Render(hostname)` appends the Debian-convention `127.0.1.1 <hostname>` line, and `Write(path, hostname)` writes `Render`'s output atomically (temp file + rename), the same pattern and rationale as `netup.WriteResolvConf` (gosd-init's rootfs is the initramfs's own writable tmpfs-like mount; no fsync needed since nothing here survives a reboot). Kept intentionally free of `internal/initramfs`'s cpio/zstd dependency so importing it into the minimal gosd-init binary costs nothing.
- `internal/pipeline.Assemble` now bakes `/etc/hosts` (mode 0644, `hostsfile.Static()`) into every image's initramfs alongside `/init`/`/app`/`config.json`, and includes it in the `ComputeIdentity` payload (`internal/initcfg/identity.go`'s docstring updated to name it) so it participates in the image identity like every other initramfs member.
- `boot.Deps` gained `WriteHosts func(hostname string) error` (nil-checked, like the other optional deps). `boot.Run` calls it exactly once per boot, right after the provisioning-snapshot restore block — the last point `cfg.Hostname` can still change (gosd.toml/cloud-init have already had their say, and a first-boot-after-reflash snapshot restore, if any, has too) — so a reflashed board's `/etc/hosts` never disagrees with the hostname `sethostname(2)` actually set. A write failure is logged and never fatal, mirroring `applyHostname`'s own failure handling.
- `cmd/gosd-init/main.go` wires `WriteHosts` to `hostsfile.Write(hostsfile.Path, hostname)`.
- Confirmed (and recorded in `internal/hostsfile`'s package doc, rather than shipping the file) that Go's pure resolver defaults to files-then-DNS order when `/etc/nsswitch.conf` is absent — `net/conf.go`'s `lookupOrder` returns `hostLookupFilesDNS` when the nss lookup for "hosts" errors as not-exist — so no nsswitch.conf is needed for this to work.
- Tests: `internal/hostsfile`'s own unit tests (static content, rendering, atomic write, static lines surviving a rewrite over baked content); `internal/pipeline`'s `TestAssembleBakesStaticHostsIntoInitramfs`; `boot`'s `TestRunWritesEtcHostsOnceWithTheFinalSettledHostname`, `TestRunWriteHostsFailureIsNotFatal`, and `TestRunWritesEtcHostsWithProvisioningSnapshotRestoredHostname`.
- `docs/runtime.md`'s networking section gains a bullet documenting both halves (static localhost resolution, and the device's own hostname via 127.0.1.1).
- All quality gates green: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`.
