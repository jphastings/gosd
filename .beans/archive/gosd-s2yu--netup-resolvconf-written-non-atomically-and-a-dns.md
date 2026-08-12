---
# gosd-s2yu
title: 'netup: resolv.conf written non-atomically, and a DNS-less renewal ACK wipes working nameservers'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:52:39Z
updated_at: 2026-08-01T21:52:23Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`WriteResolvConf` (cmd/gosd-init/internal/netup/resolvconf.go:34-44) uses
`os.WriteFile` (O_TRUNC then write) — observably empty mid-write — and
`onLeaseFor` calls it on every lease including renewals with whatever
`ack.DNS()` returned; an ACK omitting option 6 yields an empty list and a
comment-only resolv.conf.

**Failure scenarios:** (a) Go's resolver re-reads resolv.conf ~every 5s;
a lookup during the truncate window finds no nameservers and fails — once
per renewal, forever. (b) Gateways that only include option 6 in the
initial ACK (a real embedded-router behavior on RENEW) get their working
DNS config replaced with an empty file at first renewal: DNS dies while
the network-up marker stays present.

**Fix:** write temp + fsync + rename (atomic on the RAM rootfs), and skip
the write entirely (log it) when the new DNS list is empty rather than
clobbering a valid one.

## Summary of Changes

- `WriteResolvConf` (cmd/gosd-init/internal/netup/resolvconf.go) now
  writes to `path+".tmp"` and `os.Rename`s it over `path`. No fsync:
  the doc comment explains why — gosd-init's rootfs is RAM-backed tmpfs,
  gone entirely on reboot, so there is no crash-durability property to
  buy (unlike the FAT-backed `/data` partition, where a rename alone
  leaves a ~30s window before the new directory entry is durable). Only
  the rename's *atomicity* matters here, to stop a concurrent reader
  (Go's resolver polls resolv.conf ~every 5s) from ever observing a
  truncated/empty file — and a plain rename on tmpfs already gives that
  for free.
- Empty `dns` no longer writes a comment-only file over a working one:
  `WriteResolvConf` returns a new sentinel, `ErrNoDNSServers`, and
  leaves any existing file untouched. Chose a sentinel error over adding
  a bool return or a logger parameter, per the bean's steer toward
  minimal signature/caller churn — `Deps.WriteResolvConf`'s
  `func(dns []net.IP) error` shape is unchanged in both netup.go and
  wifiup.go, so main.go's wiring needed no changes at all. The two
  `onLeaseFor` call sites (netup/netup.go, wifiup/lease.go) each gained
  an `errors.Is(err, ErrNoDNSServers)` branch on their existing
  already-logging error path, logging the skip distinctly from a real
  write failure — the minimum caller edit the bean asked for.
- Kept the diff inside resolvconf.go/resolvconf_test.go plus those two
  onLeaseFor call sites, per the scoping note against gosd-akk4 (marker
  refcounting, in parallel in another worktree touching netup.go's link
  handling, wifiup.go, and main.go) — no Deps struct or main.go changes,
  so the two PRs' diffs shouldn't meaningfully collide.
- Behavioral tests added in resolvconf_test.go:
  `TestWriteResolvConfConcurrentReaderNeverSeesEmptyFile` (writer loop
  racing a reader goroutine on the same path via `-race`-clean
  goroutines, asserting every read is non-empty) and
  `TestWriteResolvConfWithEmptyDNSLeavesExistingFileIntact` (seeds a
  file, calls with `nil` DNS, asserts both the sentinel error and the
  original bytes are untouched, and that no stray `.tmp` file is left
  behind). Existing `TestWriteResolvConfListsEachNameserver` and
  `TestWriteResolvConfOverwritesExistingContent` cover the normal-write
  path unchanged.

Gates run: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty),
`golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...` — all
clean.
