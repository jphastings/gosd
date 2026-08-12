---
# gosd-7m9y
title: The provisioning snapshot in /data is unauthenticated, so a planted one survives reflash and re-provisions the device
status: todo
type: bug
created_at: 2026-08-12T04:15:07Z
updated_at: 2026-08-12T04:15:07Z
---

**Severity: High.** Defeats reflashing — the owner's primary and most drastic
remediation — and can silently re-point a device's WiFi and public tunnel.

## Verified

`cmd/gosd-init/internal/provsnapshot/provsnapshot.go:676`:

```go
if got := digest(tomlData); got != meta.GosdTomlSHA256 {
```

A plain, **unkeyed** SHA-256 of the snapshot's own gosd.toml, stored in
`snapshot.json` right beside it. Its documented purpose (package doc,
`:26-31`) is detecting a **torn write**. It cannot detect a forged one:
anyone who can write both files computes a consistent pair offline.

`heal()` (`:312-330`) skips the restore only when
`in.Identity == snap.Identity`. It therefore fires **precisely on a
reflash** to any different or updated image.

`planRestore` then restores hostname, WiFi SSID + passphrase, the whole
`[ingress.cloudflared]` / `[ingress.tailscale-funnel]` table (token and
authkey included), and `[env]` values, writing them back to the boot
partition (`heal`, `:361`) and applying them to the running boot.

## Who can write /data

Two realistic actors, and the second matters most:

1. Physical access to the card, or to the board over USB mass storage — the
   shipped `examples/usbwebsite` shares the data partition read-write (see
   the USB mass-storage bean; the two chain).
2. **A compromised `/app`.** It runs as root and `/data` is its own storage.
   So an app compromise can plant a snapshot and persist its configuration
   across the reflash the owner performs to clean up.

## Attack

Attacker plants `/data/.gosd/provision-snapshot/{gosd.toml,snapshot.json}`
with `Identity` set to the currently-running image's, a correctly-computed
digest, and `Effective.Wifi = {SSID: "evil-ap", ...}` or an
attacker-controlled `[ingress.cloudflared]` token.

Months later the owner reflashes with a new image, believing that resets
provisioning. `heal()` sees a different identity, finds nothing fresher on
the new card, and restores the attacker's WiFi network or tunnel token. The
device joins the attacker's network, or opens a tunnel the attacker
controls, having just been "factory reset". `/data` is untouched by an
Imager reflash, and cannot be cleared from a macOS host at all when it is
ext4.

## Fix — be honest about what is achievable

A keyed MAC has nowhere to keep its key: the boot partition is wiped by the
reflash this must survive, `/data` is what the attacker reads, and these
boards have no TPM or secure element. So do not file this as "add HMAC".
The practical measures, in order of value:

1. **Do not silently restore secrets.** Restoring an `[ingress.*]` token or
   authkey from unauthenticated storage is the highest-value part to close.
   Either drop `[ingress.*]` from the restore set, or gate it behind an
   explicit opt-in in the freshly-flashed gosd.toml.
2. **Validate everything restored**, to the same standard as the on-card
   path — the hostname gate is missing today (sibling bean).
3. **Log loudly** what was restored and from where, so a restore is visible
   in the console rather than silent.
4. **Document that `/data` is a trust boundary** and that a plain reflash
   does not clear provisioning. Users reasonably assume the opposite, and
   `docs/design/upgrade-path.md`'s adoption-gate language encourages that
   assumption.

## Todos

- [ ] Decide and record whether `[ingress.*]` is restorable at all from an unauthenticated snapshot
- [ ] Validate every restored field with the same gates the on-card path uses
- [ ] Log each restored key at boot
- [ ] Document /data as a trust boundary that survives reflash, in docs/design/upgrade-path.md and the provsnapshot package doc
- [ ] Correct the package doc, which presents the digest as an integrity control without stating it is not an authenticity control
