---
# gosd-lbpm
title: 'wifiup: support hidden SSIDs (directed probe)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-06T02:24:10Z
updated_at: 2026-07-06T09:22:54Z
parent: gosd-ko20
---

Gap surfaced by the provisioning parser (PR #30): Imager lets end users mark a WiFi network as hidden, and internal/provision parses hidden:true onto WifiNetwork.Hidden — but wifiup joins via ordinary scan results, so a hidden (non-broadcasting) SSID will never be found and the join loops forever. Fix in wifiup: when Hidden is set, use a directed/active scan for the specific SSID (nl80211 scan request with SSID attribute via mdlayher/wifi, or attempt Connect without a prior scan match if the API allows). Log clearly when waiting on a hidden network. Testable with fakes for the state machine; real verification on the Pi bench with a hidden test AP.

## Acceptance
Fake-driven test showing a Hidden network triggers the directed path; bench: Pi Zero 2W joins a hidden test network provisioned via Imager.


## Summary of Changes

Threaded `Hidden` through the credential chain and wired directed-join
logging, without changing the actual join mechanism:

- `wifiup.Credentials` gained a `Hidden bool` field.
- `ConfigCredentials.Credentials()` now carries `Hidden` from
  `provision.WifiNetwork` alongside SSID/passphrase whenever a Provision
  entry is the effective source, and clears it when `gosd.toml` (which has
  no hidden concept — schema locked) overrides Provision. `initcfg.Wifi`
  (config.json) likewise has no hidden concept, so that source is always
  `Hidden: false`. Existing SSID/passphrase precedence (gosd.toml > cloud-init
  Provision > config.json) is untouched.
- `wifiup.Run` logs `hidden SSID %q: probing directly; this can take
  longer` once, right before starting the association loop, when
  `creds.Hidden` is set.

### Technical approach (per the bean's options)

Checked `github.com/mdlayher/wifi`'s pinned v0.8.0 source directly
(`client_linux.go`): its only scan entry point, `Client.Scan`, hardcodes an
empty/wildcard SSID into `NL80211_ATTR_SCAN_SSIDS` with no parameter to
target a specific SSID — there is no directed-scan-by-SSID API to call, so
option "contribute the nl80211 scan-SSID attribute via the library's raw
netlink access" would mean carrying a local patch/fork with no upstream path
evaluated yet.

More importantly, `wifiup` never scanned before joining in the first place
— `associate()` goes straight to `WifiClient.Connect`/`ConnectPSK`, both of
which issue `NL80211_CMD_CONNECT` with the target SSID directly (no
scan-match gate to satisfy for a hidden or a visible network alike). So
option (a), "attempt Connect directly without a prior scan match," isn't a
new code path to add — it's what the join loop already does, and the
gap was purely that `Hidden` never reached `Credentials` and there was no
"this may take longer" signal.

Evidence this works with brcmfmac firmware SME: brcmfmac's
`brcmf_cfg80211_connect` (drivers/net/wireless/broadcom/brcm80211/brcmfmac/cfg80211.c)
copies `sme->ssid`/`ssid_len` straight into the extended join parameters
(`ext_join_params->ssid_le`) and sets active-scan parameters
(`scan_le.active_time`, `scan_le.nprobes`) on the same request, handed to
firmware via the `"join"` `bsscfg` iovar — i.e. the firmware performs its
own active/directed probe for exactly that SSID as part of association,
regardless of whether the host previously saw it in a passive scan. Falls
back to the simpler `BRCMF_C_SET_SSID` command on older firmware that
lacks the extended-join iovar, with the same SSID-direct-join behavior.
This is full-MLME-offload behavior (brcmfmac hands scan+auth+assoc to
firmware), consistent with why NetworkManager/Android join hidden networks
against this driver without a preceding host-side scan step.

### Deviations from the bean text

- No new nl80211 attribute-encoding code was added to `platform_linux.go`:
  the existing `Connect`/`ConnectPSK` calls already send the SSID directly
  in `CMD_CONNECT`, and per the evidence above that's sufficient for
  brcmfmac. Nothing to add there.
- `COMPATIBILITY.md`'s hidden-SSID row moved to ✅, matching the table's
  documented convention that ✅ means "code-complete, unit-tested, not yet
  hardware-verified" (see the table's own preamble) rather than a new status
  tier.

### Still open

Bench verification (Pi Zero 2W joining a real hidden test AP provisioned via
Imager) is unchecked; bean stays in-progress until that's done.
