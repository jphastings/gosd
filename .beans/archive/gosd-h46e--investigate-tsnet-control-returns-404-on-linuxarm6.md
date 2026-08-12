---
# gosd-h46e
title: 'Investigate: tsnet control returns 404 on linux/arm64 board but 200 on macOS (same key/code)'
status: completed
type: bug
priority: high
created_at: 2026-08-08T11:51:27Z
updated_at: 2026-08-08T11:51:27Z
---

Bench (nanopi-zero2, 2026-08-08). After the three gosd-6cf2 shim bugs were
fixed, tsnet on the board attempts registration cleanly but Tailscale control
returns `tsnet.Up: 404 Not Found` at doLogin(regen=true). NOT reproduced off
the board — the identical shim binary + same reusable auth key registers
successfully on macOS/arm64 (RegisterReq: machineAuthorized=true, tailnet IP
assigned).

## Exhaustively ruled out (all verified on-device this session)
- Auth key: valid + reusable (works from macOS AND `tailscale up` on JP's laptop).
- Clock: NTP-synced to correct time seconds BEFORE each attempt.
- DNS: NTP resolves the pool.ntp.org HOSTNAME and syncs; controlplane resolves
  to real Tailscale IPs (192.200.0.0/24, 2606:b740:49::/…).
- Egress/TLS/CA: shim preflight `GET https://controlplane.tailscale.com/key?v=110`
  from the board = 200 OK, valid cert against the shipped Mozilla bundle. Port 80
  = normal 302→https.
- Code logic: same binary works on macOS.
- Stale /data state: force-removing tailscaled.state every boot did not help.

## The remaining anomaly
Standard `http.Get` to /key from the board = 200, but tsnet's OWN control client
= 404, on the same board at the same moment; and the same tsnet code = 200 on
macOS. Verbose tsnet Logf on the board never prints "control server key from
controlplane…" before the 404, whereas macOS does. So tsnet's control fetch
fails on linux/arm64 in the gosd-init environment specifically.

## Leading hypotheses to test NEXT (off the physical bench where possible)
1. Reproduce with the linux/arm64 shim under qemu-virt or a linux/arm64
   container (same gosd-init-like minimal env: only TS_AUTHKEY+TS_LOGS_DIR, no
   HOME, initramfs-style rootfs) — cheaper than reflashing, and tells us if it's
   linux-vs-darwin or board-specific.
2. tsnet uses `controlhttp` (a custom dialer + Upgrade handshake, NOT plain
   http.Get). Instrument/trace controlhttp's actual request+response on linux:
   the 404 is on tsnet's control path, which differs from http.Get. Suspect the
   noise/Upgrade request or an HTTP/2/ALPN/dialer difference.
3. Check whether the minimal child env (no HOME, no PATH, minimal /etc) changes
   tsnet's control dialer behavior on linux (e.g. resolver mode, bootstrap DNS).
4. Serial-drop control: at 1.5M the DIAG burst may drop the "control server key"
   line; re-run with Logf filtered to ONLY control lines to confirm tsnet truly
   never fetches the key vs the line being lost.

## Serial capture gotcha seen again
The 1.5M CP2102N capture + disk-full killed readers mid-boot and truncated logs;
keep one reader, keep disk clear, and prefer qemu repro for log fidelity.

Blocks gosd-79v8 (bench sign-off) but NOT gosd-6cf2 (the three real fixes).

## Root cause & fix (2026-08-09, bench nanopi-zero2)

**Root cause: the shim's `ts_omit_*` feature-trim tags.** `gosd build --ingress
tailscale-funnel` compiled the shim with 74 `ts_omit_*` tags
(`internal/build/tsfunnel.go`, `tsfunnelOmitTags`, gosd-65uy decision 2). One
of them broke tsnet's control-plane registration: the trimmed shim fell into a
keyless interactive login (`StartLoginInteractive` → `doLogin(regen=true,
hasUrl=false)`) that the coordination server answered with **404**, ~30ms in,
before it ever sent `RegisterReq`. So the node never joined the tailnet and
Funnel never opened.

**Why it never reproduced off-bench** (correcting this session's CLAUDE.md
triage note): it isn't linux/arm64- or environment-specific at all. Every
off-bench test — darwin, a linux/arm64 container (full AND minimal env), and
qemu-virt running the real gosd initramfs+init — built an **un-trimmed** binary
(`go build`), and all registered fine. Only `gosd build` applies the trim, so
only the real shipped shim hit it.

**How it was isolated, all on the same board/key/network:** a plain `go build`
tsnet harness registered; the `gosd build` shim 404'd. Ruled out one variable
at a time on-device — DNS resolves controlplane to real Tailscale anycast IPs
and `GET /key` = 200; clock NTP-correct before start; `TS_AUTHKEY` present in
the shim env (`len=62`); setting `srv.AuthKey` explicitly still 404'd; tmpfs
state dir instead of vfat `/data` still 404'd; re-ran the harness as a control
→ still registers (not rate-limiting). Clearing the omit tags → the shim
registered, opened Funnel, and served the app publicly.

**Fix (this PR):** drop the `ts_omit_*` trim entirely — the shim ships full
tsnet (kept `-ldflags="-s -w"`, the exact config proven on hardware). ~30MB of
RAM-resident initramfs, deemed well worth guaranteed registration (JP,
2026-08-09). The "no interactive surface" property no longer rests on
compiling SSH out; it holds because the shim only calls `ListenFunnel` and
never enables tsnet's SSH server.

**Proven end to end on nanopi-zero2:** `curl https://funneltest.elk-dinosaur.ts.net/`
returned the hello app's response through Funnel.

Follow-up (optional, not blocking): if the ~30MB matters later, bisect the tag
set to find the single load-bearing tag and re-introduce the safe remainder —
doable in a linux/arm64 container (registration reproduces there), no bench
needed.
