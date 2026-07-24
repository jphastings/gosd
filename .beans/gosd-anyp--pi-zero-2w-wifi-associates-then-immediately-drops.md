---
# gosd-anyp
title: Pi Zero 2W WiFi associates then immediately drops in a loop (WPA2)
status: todo
type: bug
priority: normal
created_at: 2026-07-24T19:18:43Z
updated_at: 2026-07-24T19:49:06Z
---

Found during Pi Zero 2W first hardware boot (gosd-m9dj, 2026-07-24), immediately after the DTB fix (gosd-f59k) got the board booting. wifiup associates with the WPA2-Personal AP then loses association within ~1s, looping indefinitely ('wlan2 associated with ...' / 'lost its WiFi association; reconnecting'). Classic firmware-side 4-way-handshake-failure signature: wifiup hands the derived PSK to brcmfmac via nl80211 CONNECT and the AP rejects the key exchange.

Ruled out so far: brcmfmac firmware set complete (bin+nvram txt+clm_blob all present per manifest, zero kernel errors); PSK derivation verified correct against the IEEE 802.11 test vector ('password'/'IEEE' → f42c6fc5...) — note the existing credentials_test.go asserted DerivePSK against ITSELF (want derived by the function under test), so the derivation was previously unpinned; a ground-truth vector test now exists (uncommitted vector_check_test.go in the wifiup package — fold into this bean's PR).

Open leads (JP checking at time of filing): passphrase as actually written into gosd.toml by the build's --wifi-pass (shell-quoting risk); router possibly WPA2/WPA3 transitional with PMF quirks (locked decision: WPA2-PSK only, WPA3 out of scope but must be LOGGED clearly — currently nothing detects/logs a transitional/PMF mismatch).

Improvements this bean should land regardless of root cause:
1. Log the nl80211 disconnect/deauth REASON CODE in the 'lost its WiFi association' message — reason 15 (4-way timeout) vs 2 vs others discriminates instantly and would have saved this session real time.
2. Commit the IEEE-vector DerivePSK test (replacing the self-referential assertion).
3. Curiosity to explain: interface enumerated as wlan2 (not wlan0) on a single-radio board with no driver reloads visible.
4. If transitional/PMF is implicated: detect and log clearly per the WiFi-scope locked decision, and decide whether CONNECT attrs need MFP-capable set.

**Root-cause hypothesis firmed up (2026-07-24):** JP's router is a Netgear Nighthawk RAXE300 — WiFi 6E, tri-band single-SSID. Passphrase verified character-exact in gosd.toml; UI says 'WPA2 Personal'. But 6 GHz mandates WPA3-SAE+PMF, so single-SSID 6E routers beacon WPA3-transition with PMF capable/required on all bands while the UI still displays WPA2 — and a plain WPA2-PSK nl80211 CONNECT with no MFP attribute gets association-then-key-exchange-bounce, exactly the observed loop.

Fix direction: declare NL80211_ATTR_USE_MFP = MFP_CAPABLE in wifiup's CONNECT — PMF negotiation when the AP wants it, harmless on plain WPA2 APs, still within the WPA2-only locked scope (802.11w ≠ WPA3). Discriminating experiment in progress: WPA2-only guest SSID + gosd.toml hand-edit (also the first hardware exercise of the documented end-user hand-edit provisioning fallback).
