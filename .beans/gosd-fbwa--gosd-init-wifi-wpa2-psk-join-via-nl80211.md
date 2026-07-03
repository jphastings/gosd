---
# gosd-fbwa
title: 'gosd-init WiFi: WPA2-PSK join via nl80211'
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-03T16:59:34Z
parent: gosd-ko20
blocked_by:
    - gosd-kkz4
    - gosd-vtce
---

Join a WPA2-PSK network with no wpa_supplicant, using the brcmfmac firmware SME.

Locked approach: github.com/mdlayher/wifi — wait for the wlan interface to appear (firmware blob loads from /lib/firmware at driver probe; can take seconds), then Connect with the passphrase-derived PSK. Compute PSK from passphrase via PBKDF2 (golang.org/x/crypto/pbkdf2, 4096 iterations, ssid as salt) when given a plaintext passphrase; accept a pre-hashed 64-hex PSK directly. Open networks: plain Connect. WPA3/EAP: out of scope, log unsupported.

Behavior: retry scan+connect forever with backoff (AP may be down at boot); after association run the same DHCPv4 path as the Ethernet task (share code); on deauth/disconnect events, reconnect. Credentials come from config.json in v0.1 (Imager provisioning replaces the source in v0.2 — keep the credential source behind a small interface).

- [ ] PSK derivation with test vectors (use the well-known IEEE test vectors for PBKDF2-SHA1 WPA)
- [ ] Connect/reconnect loop
- [ ] Verify against real hardware on the Pi bring-up task; note firmware-load-to-join timing here

## Acceptance
Pi Zero 2W joins the test WPA2 network from baked credentials and gets a lease; survives AP reboot without device reboot.
