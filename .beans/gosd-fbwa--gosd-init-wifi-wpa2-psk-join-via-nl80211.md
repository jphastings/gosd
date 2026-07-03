---
# gosd-fbwa
title: 'gosd-init WiFi: WPA2-PSK join via nl80211'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-03T23:55:50Z
parent: gosd-ko20
blocked_by:
    - gosd-kkz4
    - gosd-vtce
---

Join a WPA2-PSK network with no wpa_supplicant, using the brcmfmac firmware SME.

Locked approach: github.com/mdlayher/wifi — wait for the wlan interface to appear (firmware blob loads from /lib/firmware at driver probe; can take seconds), then Connect with the passphrase-derived PSK. Compute PSK from passphrase via PBKDF2 (golang.org/x/crypto/pbkdf2, 4096 iterations, ssid as salt) when given a plaintext passphrase; accept a pre-hashed 64-hex PSK directly. Open networks: plain Connect. WPA3/EAP: out of scope, log unsupported.

Behavior: retry scan+connect forever with backoff (AP may be down at boot); after association run the same DHCPv4 path as the Ethernet task (share code); on deauth/disconnect events, reconnect. Credentials come from config.json in v0.1 (Imager provisioning replaces the source in v0.2 — keep the credential source behind a small interface).

- [x] PSK derivation with test vectors (use the well-known IEEE test vectors for PBKDF2-SHA1 WPA)
- [x] Connect/reconnect loop
- [ ] Verify against real hardware on the Pi bring-up task; note firmware-load-to-join timing here

## Acceptance
Pi Zero 2W joins the test WPA2 network from baked credentials and gets a lease; survives AP reboot without device reboot.

## Summary of Changes

Implemented `cmd/gosd-init/internal/wifiup`, a new package bringing up WiFi networking after `/app` starts, following the `netup`/`boot` packages' style exactly: a pure, fake-tested state machine (`wifiup.go`, `credentials.go`, `psk.go`, `lease.go`) behind thin interfaces (`WifiClient`, `CredentialSource`), with the real nl80211-backed implementation in `platform_linux.go` (`linux` build tag) and a stub in `platform_other.go` for macOS.

- `wifiup.Run` waits (patiently, with backoff) for a wlan-station interface to appear, does nothing at all if no WiFi credentials are configured (`ConfigCredentials.Credentials()` returns `ok=false` for an empty SSID — an Ethernet-only board never spins a retry loop), then associates and reconnects forever, handing the interface to `netup.RunDHCP` once associated.
- Association loss is detected by polling `WifiClient.Associated` every 3s (mdlayher/wifi exposes no deauth/disconnect netlink event stream, only request/response commands), which cancels the DHCP context and triggers a fresh scan+connect with backoff.
- `PSK` derivation (`psk.go`): `DerivePSK` uses the standard library's `crypto/pbkdf2` (available at this module's Go version, so no `golang.org/x/crypto` dependency needed for it) — verified against the well-known IEEE 802.11i-2004 Annex H.4 PBKDF2-SHA1 test vectors. `ParsePSKHex` accepts a pre-hashed 64-hex-character PSK directly. `ConfigCredentials` (config.json's `wifi.ssid`/`wifi.passphrase`) distinguishes the two forms by shape (64 valid hex chars vs. anything else), without any config.json schema change.
- `boot.Deps.StartNetworking`'s signature grew one parameter (`cfg initcfg.Config`) so `main.go` can build WiFi credentials from the already-parsed, cmdline-override-merged config without re-reading config.json; this is the only change to the `boot` package (plus updating the one call site and its test).
- `main.go` now starts both `netup.Run` (Ethernet, unchanged) and `wifiup.Run` (WiFi) from `StartNetworking`; if `wifiup.NewPlatform()` fails to open nl80211 at all (no WiFi hardware/driver — the expected case on an Ethernet-only board), it logs and skips WiFi entirely rather than treating it as fatal.

### Deviation: WPA2-PSK connect bypasses the library's own PBKDF2 step

`github.com/mdlayher/wifi`'s public `ConnectWPAPSK(ifi, ssid, psk string)` always re-derives the PMK internally via its own PBKDF2-SHA1 from whatever string is passed — there is no public API to hand it an already-derived key. That's incompatible with "accept a pre-hashed 64-hex PSK directly" actually being usable to *join* a network (the entire point of accepting one is that no plaintext passphrase need ever exist on the image, e.g. for v0.2 Imager provisioning). To honor that, `nlClient.ConnectPSK` (in `platform_linux.go`) issues the same `NL80211_CMD_CONNECT` nl80211 request the library's `ConnectWPAPSK` does — same attributes, same cipher/AKM suites — over its own short-lived generic netlink connection, substituting the already-resolved PMK bytes directly instead of the library's internal derivation step. This means gosd-init's own `DerivePSK`/`ParsePSKHex` is the single source of truth for the PMK regardless of which form config.json supplied, and there's exactly one connect code path for both. It also means this specific code path (~40 lines of manual netlink attribute encoding, mirrored attribute-for-attribute from the library's own source) is new, untested-by-upstream code, flagged below for hardware validation alongside everything else nl80211-related in this bean.

### Honesty: hardware validation not done

Per the bean's own scope, "Verify against real hardware on the Pi bring-up task; note firmware-load-to-join timing here" is left unchecked — there is no Pi Zero 2W available in this environment. Everything here (interface-wait timing, actual nl80211 CONNECT behavior including the raw-PMK path above, brcmfmac firmware SME behavior, and disconnect-detection-by-polling) is exercised only via fakes and passes `go build`/`go vet`/`golangci-lint` for `GOOS=linux GOARCH=arm64`, but has had zero real-hardware exercise. The bean stays `in-progress` for this reason; the Pi bring-up task is expected to validate it and report back (including the firmware-load-to-join timing this bean asked to have noted here).
