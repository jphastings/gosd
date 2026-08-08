---
# gosd-6cf2
title: 'tsfunnel shim: 3 hardware-only startup bugs (logs-dir panic, empty-state wedge, Close-masks-error)'
status: completed
type: bug
priority: high
created_at: 2026-08-08T09:35:37Z
updated_at: 2026-08-08T09:35:37Z
---

Found on the bench (nanopi-zero2, 2026-08-08) while verifying gosd-79v8:
cmd/gosd-tsfunnel panicked at startup and could never bring a tunnel up.
Three distinct, only-visible-on-hardware defects, each masking the next —
the fake-driven tests can't reach them because they never exec the real
tsnet in the real gosd-init environment (no HOME, initramfs rootfs, /data
state surviving reflash). All three fixed in cmd/gosd-tsfunnel/main.go;
proven on-device (tsnet then reached NeedsLogin -> StartLoginInteractive
and attempted real registration).

## Bug 1 — panic: "no safe place found to store log state"
tsnet's logpolicy.LogsDir probes, in order: $TS_LOGS_DIR, $STATE_DIRECTORY,
/var/lib/tailscale (MkdirAll), then os.UserCacheDir() — and PANICS if none
work. On our images none do: no /var/lib in the initramfs rootfs, and the
supervised child has no HOME so UserCacheDir fails. cloudflared dodged the
analogous problem because its wiring sets HOME=/run/gosd/cloudflared; the
tsfunnel wiring set no equivalent. Fix: shim sets TS_LOGS_DIR to the state
dir (writable, on /data) after MkdirAll (LogsDir requires it to pre-exist).

## Bug 2 — permanent wedge: "unexpected end of JSON input"
tsnet writes tailscaled.state and tailscaled.log.conf as plain JSON with no
write->rename, so a power cut mid-write leaves an empty/truncated file.
tsnet.Up then refuses to load it and NEVER regenerates — a permanent wedge,
made STICKY because /data survives reflash (a plain Imager reflash cannot
clear it; the host can't even mount ext4 GOSD-DATA to delete it). Fix: the
shim drops either file when it exists but isn't valid JSON, before Up. Both
regenerate when absent; the log ID is unshipped telemetry (Logf discarded)
and an unparseable state file holds no identity worth keeping (fresh
registration via TS_AUTHKEY is the correct recovery). A VALID state file is
left untouched, preserving node identity across reboot and reflash.

## Bug 3 — panic: "unreachable" masking every real Up error
The shim's `defer srv.Close()` ran on the Up-failure path too; tsnet's Close
panics ("unreachable" from tsdial.PeerAPIHTTPClient) when Up failed before
the dialer initialized, crashing the process and HIDING the real Up error
(bugs 1 and 2 were both invisible behind this until fixed). Fix: only
defer srv.Close() AFTER a successful Up; on failure return the wrapped error
and let process exit reclaim resources.

## Todos
[ ] Land the three fixes in cmd/gosd-tsfunnel/main.go (done on bench, needs PR)
[ ] Unit tests: corrupt/empty tailscaled.state and tailscaled.log.conf are
    removed while a valid state file is preserved; TS_LOGS_DIR set + dir
    created; no Close on the Up-failure path (table-drive the heal)
[ ] Consider whether the wiring (cmd/gosd-init/internal/tsfunnel) should set
    TS_LOGS_DIR in the child env instead of the shim (cloudflared-HOME
    parallel) — decide one owner, note it
[ ] Upstream note for gosd-e721: tsnet should write these state files
    atomically (write->rename) and/or self-heal an unparseable one rather
    than wedging; prepare the patch/rationale, do not PR upstream without JP
[ ] Re-verify on-device once a valid auth key registers a tunnel (gosd-79v8)

## Note
Not the cause of the final 404: with all three fixed, tsnet cleanly reached
registration and Tailscale's control plane returned 404 Not Found for the
supplied auth key (clock was already NTP-correct — ruled out). That is a
credentials issue tracked in gosd-79v8, not a code defect.

---

## Update (bench, 2026-08-08): the 404 is NOT gosd — Mac reproduction proves it

After the three fixes, tsnet on the board reaches registration cleanly but
Tailscale control returns `tsnet.Up: 404 Not Found`. Bisected it:

- **Same shim binary (same source), same auth key, run on this Mac
  (darwin/arm64): SUCCESS** — `RegisterReq: got response;
  machineAuthorized=true`, tailnet IP assigned. So the key is valid+reusable,
  tsnet works, the code is correct, the version is fine.
- **Board (linux/arm64) with verbose tsnet Logf**: never logs
  `control server key from https://controlplane.tailscale.com` /
  `RegisterReq` at all — the FIRST HTTPS GET to controlplane 404s, before any
  control-server-key exchange.
- Ruled out on the board: clock (NTP-synced to correct time before the
  attempt), DNS (NTP uses the pool.ntp.org HOSTNAME and synced, so resolution
  works), egress+UDP, CA roots (shipped by gosd-kzgq; a TLS failure would be a
  TLS error, not an HTTP 404).

**Conclusion:** the board's HTTPS request to controlplane.tailscale.com is
being answered with 404 by something on the path that the Mac (same LAN,
same key) does not hit. This is a network/infrastructure condition specific
to the board's traffic, not a gosd defect. Next diagnostic if it recurs: a
shim preflight `http.Get("https://controlplane.tailscale.com/key?v=NN")`
logging status/error, to confirm board-side HTTPS-to-control in isolation.
The three code fixes above stand on their own and should still land.

---

## Correction (bench, 2026-08-08): the 404 is NOT the network either

The earlier "network/infrastructure" conclusion was WRONG. On-device preflight
(net.LookupHost + http.Get from the shim, before tsnet) proved the board's own
HTTPS path to control is healthy:
- controlplane.tailscale.com resolves to real Tailscale IPs (192.200.0.0/24
  + 2606:b740:49::/…);
- a plain `GET https://controlplane.tailscale.com/key?v=110` from the board
  returns **200 OK** with a valid cert (Mozilla roots, shipped by gosd-kzgq);
- port 80 returns a normal 302→https (not a 404) from the same LAN.

Yet tsnet's OWN control client on the board still returns `tsnet.Up: 404 Not
Found` at doLogin, and (per verbose Logf) never logs "control server key from
…" — while the identical tsnet code + same key on macOS logs it and registers
(machineAuthorized=true). Ruled out: key, clock, DNS, egress, TLS/CA,
code-logic, AND stale /data state (force-removing tailscaled.state each boot
did not help). The 404 is specific to tsnet's control HTTP path on
linux/arm64 in the gosd-init environment, cause not yet pinned. Split into its
own investigation bean (see below) so THIS bean's three confirmed+fixed bugs
can land independently.

## Summary of Changes

Fixed all three bugs in cmd/gosd-tsfunnel/main.go via a `prepareStateDir`
helper called before the tsnet.Server is built: (1) MkdirAll the state dir
then set TS_LOGS_DIR to it (kills the logpolicy.LogsDir panic); (2) drop
tailscaled.state / tailscaled.log.conf when present-but-not-valid-JSON, leaving
a valid state file untouched (self-heals the reflash-sticky empty-file wedge
while preserving node identity); (3) `defer srv.Close()` moved to AFTER a
successful Up so the failure path returns the real error instead of tsnet's
"unreachable" Close panic. New main_test.go table-tests prepareStateDir
(empty/truncated removed, valid preserved byte-for-byte, TS_LOGS_DIR set,
unrelated files untouched). Shipped in **PR #237** (13/13 CI green,
both linux/arm64 and arm/GOARM=6 cross-compiles pass). All three were proven
on the nanopi-zero2 bench 2026-08-08. The separate, still-open tsnet-404 is
gosd-h46e (NOT addressed here). Upstream write→rename/self-heal note recorded
on gosd-e721.
