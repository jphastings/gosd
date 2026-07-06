---
# gosd-pctc
title: 'Provisioning parser in gosd-init: consume Imager files from GOSD-BOOT'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-06T02:13:44Z
parent: gosd-b22t
blocked_by:
    - gosd-qvoq
    - gosd-fbwa
---

Implement `internal/provision`: at boot, gosd-init reads the mounted /boot partition and extracts hostname + WiFi credentials from whatever Raspberry Pi Imager wrote, per docs/provisioning-formats.md.

Rules:
- Parse ONLY against the committed fixtures — every extraction path has a fixture-driven test. firstrun.sh parsing is targeted regex extraction (we never execute it); custom.toml via TOML; cloud-init network-config via YAML (gopkg.in/yaml.v3)
- Precedence (locked): gosd.toml > custom.toml > cloud-init files > firstrun.sh > baked config.json
- Support plaintext passphrase AND 64-hex PBKDF2 PSK (reuse the WiFi task derivation code)
- Unknown/unparseable files: log a warning with the filename, never crash, fall through to next source
- Result feeds the credential-source interface the WiFi task defined; hostname applies before /app launch if possible (re-exec not required — set it when discovered and log)

- [x] Parser + fixture tests for every scenario captured in research
- [x] Wire into gosd-init boot sequence after /boot mount
- [ ] On-hardware test: flash with real Imager + customization, device joins WiFi (record Imager version used)

## Acceptance
End-to-end: image built by gosd with NO baked credentials, flashed via Imager with WiFi entered in the dialog, boots and joins that network.

## Re-scope (2026-07-05, supersedes the parse list above)
Per the flashing-path decision and the gosd-qvoq source-analysis findings (docs/provisioning-formats.md): custom.toml DOES NOT EXIST in rpi-imager (drop it), and firstrun.sh parsing is OUT OF SCOPE (the catalog flow declares init_format=cloudinit, so Imager writes cloud-init files). Parse: cloud-init user-data + network-config (YAML) only, plus the existing gosd.toml/config.json chain. Precedence: gosd.toml > cloud-init > baked config.json. If a firstrun.sh is detected on /boot, log one clear line pointing the user at gosd.toml instead of parsing it. PSK note: Imager writes the 64-hex PBKDF2 PSK — identical derivation to wifiup.DerivePSK, accept it directly. Still fixture-driven: blocked on gosd-qvoq bench capture (cloud-init scenarios via a custom repo).


## Summary of Changes

Implemented `internal/provision` (Read + parseUserData + parseNetworkConfig)
against the real Imager v2.0.10 fixtures in
`internal/provision/testdata/imager-2.0.10/`, per the 2026-07-05 re-scope:
cloud-init `user-data` (hostname; every other field logged-and-skipped in one
summary line) and `network-config` (netplan wifis→access-points, walked via
raw yaml.Node to preserve file order for "take all" multi-AP support; password
passed through unexamined — plaintext vs. 64-hex PSK is still distinguished
only by wifiup.ConfigCredentials, reused rather than duplicated). firstrun.sh
is detected (for one log-and-point-at-gosd.toml line) but never parsed or
executed, per the locked flashing-path decision.

Wired in: `wifiup.ConfigCredentials` gained a `Provision []provision.WifiNetwork`
field slotted between GosdToml (highest) and Wifi/config.json (lowest — locked
precedence gosd.toml > cloud-init > config.json), so wifiup's existing
PSK-vs-passphrase detection and derivation apply unchanged. `boot.Deps` gained
`ReadProvisioning`, called right alongside `ReadGosdToml` after the /boot mount;
`StartNetworking` grew a `provisionWifi []provision.WifiNetwork` parameter to
carry it through to wifiupDeps. Each winning source is logged
("hostname from cloud-init user-data", "wifi from gosd.toml", etc.).

docs/runtime.md's networking section was stale (said WiFi credentials were
baked but unused) — corrected to describe the current wired-in WiFi + the new
provisioning precedence chain. docs/provisioning-formats.md got a short
"Implementation" pointer at the tail.

Deviations from the bean body's original (superseded) parse list: no
custom.toml, no firstrun.sh regex parsing — both already ruled out by the
2026-07-05 re-scope note, not new decisions made here. Hidden-network
(`hidden: true`) is parsed and carried on `provision.WifiNetwork.Hidden` but
not yet acted on — wifiup's nl80211 Connect/ConnectPSK have no directed-probe
support for hidden SSIDs today; that's a wifiup enhancement, out of scope for
a provisioning-parser bean.

All quality gates pass (`go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` on both darwin and `GOOS=linux`), plus a
`CGO_ENABLED=0 GOOS=linux GOARCH=arm64` static build of `cmd/gosd-init`.

On-hardware verification (flash with real Imager, confirm the device joins
WiFi) is unchecked and left for a human at the bench — this bean stays
in-progress until that's done.
