---
# gosd-virc
title: 'Cloudflare Tunnel ingress: gosd build --ingress cloudflared (arm64 v1)'
status: todo
type: epic
priority: normal
created_at: 2026-08-07T12:51:17Z
updated_at: 2026-08-07T13:59:09Z
---

JP request (2026-08-07): gosd devices' HTTP services reachable from the public
internet via Cloudflare Tunnel with ZERO app code — a build flag bakes a
cloudflared binary into the image, gosd-init keeps it alive, and
`[ingress.cloudflared]` in gosd.toml declares the tunnel. Full investigated
design in the child beans; plan session record: token format, ELF staticness,
and armv6 incompatibility all verified against real release assets.

## Locked decisions (JP, 2026-08-07 planning session)

1. **gosd-init supervises cloudflared.** This amends locked decision gosd-oyhi
   with a narrow carve-out: gosd-SHIPPED system services may be
   gosd-init-supervised; USER externals stay app-owned via os/exec. The
   carve-out is recorded in gosd-oyhi + docs/runtime.md (~L825 single-child
   bullet) + boot/reaper.go L63-66 stash comment, all in the wiring PR.
2. **Locally-managed mode only in v1**: `token` + `hostname` + `port`, all
   required. Ingress rules are declared on-device (generated config.yml).
   Remote/dashboard-managed and quick tunnels are deferred; token-without-
   hostname logs an actionable "not supported yet".
3. **No credentials file.** The tunnel token IS the credentials triple: base64
   JSON {"a":AccountTag,"s":TunnelSecret,"t":TunnelID} ≡ the credentials
   file's fields (cloudflared acknowledges via `tunnel token --cred-file`).
   gosd-init decodes the token and synthesizes credentials.json + config.yml
   under /run/gosd/cloudflared/ at boot. Decoder tolerates unknown extra
   fields; errors actionably if a/s/t missing (format is de-facto stable since
   2021 but undocumented; fallback schema if it ever breaks: a
   `credentials = "<base64 of JSON>"` key). Consequence: the whole section
   round-trips through provsnapshot — ingress survives reflash with no file
   on GOSD-BOOT.
4. **Upstream prebuilt binaries**, pinned URL + sha256 (existing third-party-
   blob policy; never re-hosted). Verified: cloudflared-linux-arm64 is
   statically linked (no PT_INTERP — passes staticelf.Verify), ~25MB.
   **arm64 boards only**: the official arm asset is GOARM=7 → "illegal
   instruction" on pi-zero-w armv6 (upstream issues #1136/#1162), and GOARM
   level is undetectable in ELF headers (gosd-aur4) so this is a hard board
   check, not a staticelf matter. pi-zero-w: actionable build error +
   COMPATIBILITY.md footnote. Revisit via artifacts-CI compile if demand appears.
5. **Ingress rule targets `http://localhost:<port>`** (gosd-e3xi ships
   /etc/hosts — soft dependency SATISFIED 2026-08-07: e3xi merged;
   internal/hostsfile ships /etc/hosts in every initramfs).
6. `--no-autoupdate` always; `--loglevel warn` (info floods 115200 serial);
   `HOME=/run/gosd/cloudflared` so ~/.cloudflared probing resolves writable.
7. Locked-decision compliance: cloudflared is outbound-only (QUIC to the edge)
   and routes solely to the declared port — gosd-init gains no listener, no
   shell/SSH exists on-image ("no interactive surface" holds). Config
   precedence lock untouched: section is gosd.toml-only; config.json carries
   only the "binary is baked" bit (`ingressCloudflared`), gosd-init never
   probes the filesystem for the binary.

## Prerequisite

CA roots in every image: bean gosd-kzgq (cloudflared needs
/etc/ssl/certs/ca-certificates.crt; also introduces pipeline.ExtraFiles).

## Child-bean order

schema → build rail → runtime module → wiring+contract amendment →
provsnapshot → docs → bench verification.
