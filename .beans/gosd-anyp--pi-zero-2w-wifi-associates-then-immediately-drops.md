---
# gosd-anyp
title: Pi Zero 2W WiFi associates then immediately drops in a loop (WPA2)
status: todo
type: bug
priority: normal
created_at: 2026-07-24T19:18:43Z
updated_at: 2026-07-25T06:22:34Z
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

**Bench results (2026-07-24 evening):** the MFP_OPTIONAL attribute did NOT fix the loop; neither did removing AUTH_TYPE (exact mdlayher mirror); all 3 attribute variants × 2 APs (RAXE300 + phone hotspot) loop identically. No WPA2-only guest SSID was available to complete the discriminator. Branch `bean/gosd-anyp-mfp-capable` holds the exact-mirror revert + the IEEE-vector DerivePSK test (improvement 2, done).

**Desk research (2026-07-25) — the kernel-tree-delta theory is eliminated; the premise was inverted:**

- The failing kernel already IS the downstream tree. pi-zero-2w pins `raspberrypi/linux @ 63598c83` — an ancestor of current rpi-6.18.y (behind_by 0); `internal/kernelspec/kernelspec.go` and the released artifacts-v0.6.0 manifest.json agree. Mainline v6.18.37 is the Rockchip fleet tag only. Full-directory diff of `brcm80211/` between stable v6.18.37 and rpi-6.18.y: for a fresh WPA2-PSK offload CONNECT both trees drive an identical firmware sequence (`sup_wpa 1` → `BRCMF_C_SET_WSEC_PMK` → `SET_SSID`); `brcmf_set_pmk`, FWSUP feature detection, and all of `net/wireless/` are byte-identical. There is no downstream patch to carry — that investigation line is closed.
- gokrazy's default kernel builds from kernel.org (currently linux-7.1.4; `gokrazy/kernel.rpi` is the opt-in downstream build), so "gokrazy works on this silicon" doesn't isolate a kernel tree either. Its wifi daemon does two things wifiup doesn't: sets a regulatory region, and retries on a 15s cadence.
- Strong corroboration for the transitional-AP hypothesis above: raspberrypi/linux#4976 — 43430-class chips on downstream kernels against WPA2/WPA3-transition APs associate then deauth ("AP sees invalid keys"), with Pi Zero 2W reports as recent as 2026-07-20. Known workarounds there: `brcmfmac.feature_disable=0x82000` (disables the firmware supplicant — NOT available to wifiup, whose ATTR_PMK CONNECT requires the FWSUP-gated 4WAY_HANDSHAKE_STA_PSK feature) or `key_mgmt=WPA-PSK-SHA256` + `ieee80211w=2`. See also RH bugzilla 2302577, the hostap list thread (Aug 2024), OpenWrt forum "brcmfmac wpa3 issues", RPi forums t=370531 / t=395779.
- The neighbouring Zero 2W connecting to the same SSID is consistent with this: RPiOS uses wpa_supplicant's host-side 4-way handshake, not the firmware supplicant, so it never exercises the failing path.

**Experiment plan (updated 2026-07-25):**

1. Cheapest discriminator — no guest SSID needed: switch the phone hotspot to WPA2-only (Android: Security = WPA2-Personal; iOS: Maximize Compatibility) and retry the already-flashed card. Predicted outcome: connects.
2. gokrazy SD experiment, reinterpreted: run it against the same failing SSID and record which kernel the image uses (default `gok` build = kernel.org mainline; `kernel.rpi` = downstream — check `uname -r` or the image's config). If it ALSO fails → AP-transition × firmware-supplicant confirmed: land reason-code logging + transitional-mode detection/logging per the WiFi-scope locked decision, document "use a WPA2-only SSID", then bench-try AKM PSK-SHA256 (0x000FAC06) + USE_MFP in CONNECT before considering a host-side EAPOL fallback. If it CONNECTS → the delta is runtime, not kernel: diff regulatory-region setting, retry cadence, exact CONNECT attributes, and firmware version from dmesg; replicate in wifiup one at a time.
