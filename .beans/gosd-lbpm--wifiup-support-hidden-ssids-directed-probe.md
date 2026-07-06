---
# gosd-lbpm
title: 'wifiup: support hidden SSIDs (directed probe)'
status: todo
type: task
created_at: 2026-07-06T02:24:10Z
updated_at: 2026-07-06T02:24:10Z
parent: gosd-ko20
---

Gap surfaced by the provisioning parser (PR #30): Imager lets end users mark a WiFi network as hidden, and internal/provision parses hidden:true onto WifiNetwork.Hidden — but wifiup joins via ordinary scan results, so a hidden (non-broadcasting) SSID will never be found and the join loops forever. Fix in wifiup: when Hidden is set, use a directed/active scan for the specific SSID (nl80211 scan request with SSID attribute via mdlayher/wifi, or attempt Connect without a prior scan match if the API allows). Log clearly when waiting on a hidden network. Testable with fakes for the state machine; real verification on the Pi bench with a hidden test AP.

## Acceptance
Fake-driven test showing a Hidden network triggers the directed path; bench: Pi Zero 2W joins a hidden test network provisioned via Imager.
