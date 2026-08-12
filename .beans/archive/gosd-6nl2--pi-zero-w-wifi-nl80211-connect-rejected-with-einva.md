---
# gosd-6nl2
title: 'pi-zero-w WiFi: nl80211 CONNECT rejected with EINVAL — 43430 firmware-supplicant capability suspected'
status: completed
type: bug
created_at: 2026-07-26T04:10:40Z
updated_at: 2026-07-26T05:21:15Z
---

Found in Pi Zero W bring-up session 3 (gosd-qltr, 2026-07-26, immediately after gosd-1ey5's DMA fix let gosd-init reach wifiup). The radio probes (interface wlan1, cfg80211 up) but every ConnectPSK fails instantly with `nl80211 CONNECT: netlink receive: invalid argument` — the kernel REJECTS the request (contrast the Zero 2W saga, where CONNECT was accepted). wifiup's backoff loop and honest logging behave exactly as designed.

Leading hypothesis: the kernel gates NL80211_ATTR_PMK/WANT_1X_4WAY_HS on NL80211_EXT_FEATURE_4WAY_HANDSHAKE_STA_PSK, which brcmfmac only advertises when the firmware reports the `sup_wpa` iovar (BRCMF_FEAT_FWSUP). The Zero W's plain BCM43430 firmware (brcmfmac43430-sdio.bin — a different blob from the Zero 2W's 43436s) plausibly lacks the firmware supplicant entirely → EINVAL by design. Curiously the capture contains ZERO brcmfmac dmesg lines despite wlan1 existing (possible serial overrun or deferred firmware load) — capture the firmware version + feature state properly next bench pass (`brcmfmac` dmesg or a wifiup ext-feature probe).

Desk verification first (no hardware): (1) confirm the EINVAL gating path in the pinned tree (nl80211 CONNECT validation of ATTR_PMK vs ext feature); (2) establish whether ANY 43430 firmware build supports fwsup — check RPi-Distro/firmware-nonfree history/issues, gokrazy's Zero W (v1) WPA2 experience with mdlayher ConnectWPAPSK (their lib returns ErrNotSupported when the ext feature is absent — do Zero W gokrazy users have working WPA2?), and brcmfmac feat.c at the pin.

If the firmware genuinely cannot: this is existential for the board (WiFi is the Zero W's only network; without it the board is USB-gadget-only) and escalates to a JP design decision: implement a host-side WPA2 4-way handshake (EAPOL in gosd-init — significant feature, interacts with the locked WiFi-scope decision and the no-plaintext-passphrase design since host-side needs the PMK anyway, which we have) vs de-scope Zero W WiFi. Do not relitigate WiFi scope unilaterally — present findings and stop.

Regardless of outcome, land the small wifiup improvement: check the 4WAY_HANDSHAKE_STA_PSK ext feature before CONNECT (as mdlayher's ConnectWPAPSK does) and log an actionable "this WiFi chip's firmware lacks offloaded WPA2 handshake support" instead of a raw EINVAL retry loop.

Evidence: scratchpad qltr-boot-07-dmafix.raw (session 3). gosd-qltr blocked on this bean for its WiFi items; SD/console/boot items are all now PASSING on this board.


## Verification (2026-07-26, desk) — hypothesis OVERTURNED; no firmware wall exists

Full report: scratchpad/6nl2-fwsup-verification.md. Verdicts:

1. EINVAL gating paths confirmed at the pin (nl80211_connect L13099-102 for
   WANT_1X_4WAY_HS without 4WAY_HANDSHAKE_STA_1X; nl80211_crypto_settings
   L12107-116 for ATTR_PMK without STA_PSK) — and an audit of every other
   EINVAL branch reachable by our attribute set ruled them out.
2. brcmfmac FWSUP advertisement path confirmed (feature.c L349 sup_wpa probe;
   cfg80211.c L7709-17 sets both STA_PSK and STA_1X together).
3. **The 43430 firmware DOES support fwsup**: the pinned cyfmac43430-sdio.bin
   carries `idsup-idauth` in its build tag (downloaded + strings-verified);
   RPi-Distro/firmware-nonfree#23 names BCM43430/1 as advertising the 4WAY
   features; and our own z2wprobe boot-10 capture is bench proof of the path
   on a 43430a1 blob.

**Actual root cause (from the qltr-boot-07 capture): phantom radios.** The
armv6 kernel builds in `mac80211_hwsim` (bcmrpi_defconfig =m promoted to =y
by ModulesDisabled; no fragment disables it), creating simulated wlan0/wlan1
with no handshake features — wifiup picked hwsim's wlan1 → EINVAL by the
gates above. The REAL radio never enumerated at all: the fragment lacks
CONFIG_MMC_SDHCI_IPROC, the only driver at the pin binding the mainline DT's
`brcm,bcm2835-sdhci` WiFi-SDIO node (the defconfig's downstream bcm2835-mmc
driver matches downstream-only compatibles). This also explains the zero
brcmfmac dmesg lines. **Bonus resolution: the Zero 2W's "wlan2 curiosity"
(gosd-anyp/m9dj) is the same hwsim phantoms occupying wlan0/wlan1 — its
kernel.config also has MAC80211_HWSIM=y; its real radio simply landed at
wlan2 and got picked. Pi 3B (gosd-xhc3): no firmware wall; same hwsim trap
to avoid in its not-yet-built kernel.**

## Fix plan (kernel-config level; the EAPOL design question is MOOT)

- build/boards/pi-zero-w/kernel.fragment: `CONFIG_MMC_SDHCI_IPROC=y` (real
  radio enumerates) + `# CONFIG_MAC80211_HWSIM is not set` (kill phantoms).
- build/boards/pi-zero-2w/kernel.fragment: kill hwsim likewise (removes the
  wlan2 offset; cosmetic but real).
- build/boards/pi-3b/kernel.fragment: ensure hwsim is disabled from birth.
- wifiup: pre-CONNECT check of NL80211_EXT_FEATURE_4WAY_HANDSHAKE_STA_PSK
  with an actionable error (would have named this bug instantly). Whether
  Interfaces() should also skip phys lacking the feature (auto-pick the real
  radio) — JP's call; the fragment fix makes it moot on our own boards.
- Bench validation: rebuild zero-w kernel locally, boot: real radio as wlan0,
  brcmfmac dmesg present, association + hello.local. Then the usual
  artifacts-release batching for the fragment changes.

## Summary of Changes (implementation done 2026-07-26; bench validation pending)

- build/boards/pi-zero-w/kernel.fragment: added `CONFIG_MMC_SDHCI_IPROC=y`
  (the only driver at the pin binding the mainline DT's `brcm,bcm2835-sdhci`
  WiFi-SDIO node — without it the BCM43430 never enumerates) and
  `# CONFIG_MAC80211_HWSIM is not set` (bcmrpi_defconfig's =m was promoted
  to =y by ModulesDisabled, creating the phantom wlan0/wlan1).
- build/boards/pi-zero-2w/kernel.fragment: hwsim disabled likewise (removes
  the gosd-anyp/gosd-m9dj wlan2 offset).
- build/boards/pi-3b/kernel.fragment: hwsim disabled from birth
  (preventative; this kernel has never been built).
- wifiup pre-CONNECT guard: new WifiClient method
  `SupportsOffloadedHandshake(ifi)` — GET_WIPHY + SPLIT_WIPHY_DUMP,
  EXT_FEATURES bit-test for 4WAY_HANDSHAKE_STA_PSK, mirroring mdlayher/wifi
  v0.8.0's unexported checkExtFeature; dump flags include netlink.Request
  (the connectRequestFlags lesson). For PSK joins, an interface failing the
  check gets an actionable log naming the interface, the missing feature,
  and the phantom-radio possibility, and wifiup skips to the next capable
  candidate; sole/no-capable candidate proceeds honestly, check errors never
  skip. Open-network joins skip the check (no PMK involved). Fake-driven
  behavioral tests run on macOS. ConnectPSK's attribute set untouched.
- internal/artifacts.Version NOT bumped (tag-first, bump-second): the
  fragment changes reach real builds only after the next artifacts release.

Bench validation still pending (next zero-w session):

- [ ] Rebuild the zero-w kernel locally (`gosd build-kernel --board pi-zero-w`)
      and boot with `--artifacts-dir`
- [ ] Real radio enumerates: `mmc1` SDIO + brcmfmac dmesg lines present,
      firmware `BCM43430/1 wl0: … 7.45.98` reported
- [ ] Interface is wlan0 (no hwsim phantoms), guard passes
      (4WAY_HANDSHAKE_STA_PSK advertised)
- [ ] CONNECT accepted → association with the bench AP
- [ ] hello.local resolves over WiFi
- [ ] Then the usual artifacts-release batching for the fragment changes


## Bench validation (2026-07-26 morning) — FIX PROVEN, bean complete

Rebuilt kernel (SDHCI_IPROC=y, hwsim off, verified in emitted config; dma-
ranges patch intact). Boot: brcmfmac dmesg present for the first time on
this board (`Firmware: BCM43430/1 wl0 ... 7.45.98 (TOB)` — and the TOB blob
DID do the offloaded handshake, settling the verification caveat), real
radio as plain wlan0, `connect accepted` → lease → mDNS → hello.local
HTTP 200. The Zero W is on WiFi with GoSD's stock stack. Artifacts-release
batching carries the fragment changes to real builds (gosd-36yy/gosd-7wv9
window), same as gosd-md4w and gosd-1ey5.
