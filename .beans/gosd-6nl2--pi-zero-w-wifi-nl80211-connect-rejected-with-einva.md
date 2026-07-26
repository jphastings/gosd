---
# gosd-6nl2
title: 'pi-zero-w WiFi: nl80211 CONNECT rejected with EINVAL — 43430 firmware-supplicant capability suspected'
status: todo
type: bug
created_at: 2026-07-26T04:10:40Z
updated_at: 2026-07-26T04:10:40Z
---

Found in Pi Zero W bring-up session 3 (gosd-qltr, 2026-07-26, immediately after gosd-1ey5's DMA fix let gosd-init reach wifiup). The radio probes (interface wlan1, cfg80211 up) but every ConnectPSK fails instantly with `nl80211 CONNECT: netlink receive: invalid argument` — the kernel REJECTS the request (contrast the Zero 2W saga, where CONNECT was accepted). wifiup's backoff loop and honest logging behave exactly as designed.

Leading hypothesis: the kernel gates NL80211_ATTR_PMK/WANT_1X_4WAY_HS on NL80211_EXT_FEATURE_4WAY_HANDSHAKE_STA_PSK, which brcmfmac only advertises when the firmware reports the `sup_wpa` iovar (BRCMF_FEAT_FWSUP). The Zero W's plain BCM43430 firmware (brcmfmac43430-sdio.bin — a different blob from the Zero 2W's 43436s) plausibly lacks the firmware supplicant entirely → EINVAL by design. Curiously the capture contains ZERO brcmfmac dmesg lines despite wlan1 existing (possible serial overrun or deferred firmware load) — capture the firmware version + feature state properly next bench pass (`brcmfmac` dmesg or a wifiup ext-feature probe).

Desk verification first (no hardware): (1) confirm the EINVAL gating path in the pinned tree (nl80211 CONNECT validation of ATTR_PMK vs ext feature); (2) establish whether ANY 43430 firmware build supports fwsup — check RPi-Distro/firmware-nonfree history/issues, gokrazy's Zero W (v1) WPA2 experience with mdlayher ConnectWPAPSK (their lib returns ErrNotSupported when the ext feature is absent — do Zero W gokrazy users have working WPA2?), and brcmfmac feat.c at the pin.

If the firmware genuinely cannot: this is existential for the board (WiFi is the Zero W's only network; without it the board is USB-gadget-only) and escalates to a JP design decision: implement a host-side WPA2 4-way handshake (EAPOL in gosd-init — significant feature, interacts with the locked WiFi-scope decision and the no-plaintext-passphrase design since host-side needs the PMK anyway, which we have) vs de-scope Zero W WiFi. Do not relitigate WiFi scope unilaterally — present findings and stop.

Regardless of outcome, land the small wifiup improvement: check the 4WAY_HANDSHAKE_STA_PSK ext feature before CONNECT (as mdlayher's ConnectWPAPSK does) and log an actionable "this WiFi chip's firmware lacks offloaded WPA2 handshake support" instead of a raw EINVAL retry loop.

Evidence: scratchpad qltr-boot-07-dmafix.raw (session 3). gosd-qltr blocked on this bean for its WiFi items; SD/console/boot items are all now PASSING on this board.
