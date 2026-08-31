---
# gosd-0jl3
title: 'Build rail: gosd build --wifi-ap flag + config tree + integration test'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T05:36:15Z
parent: gosd-qfbk
blocked_by:
    - gosd-hawd
    - gosd-tufx
---

WiFi AP epic gosd-qfbk bean 4 (needs bean 1's artifact shape + bean 3's
package for field names). Blocks bean 5 (runtime wiring needs the config.json
bit this bean defines).

## Locked decisions

- `gosd build --wifi-ap` — boolean flag shaped like `--ingress cloudflared`:
  embeds the pinned hostapd binary via the existing
  `pipeline.ExtraExecutables` mechanism (the same one `--ingress cloudflared`
  already uses to land `/bin/cloudflared`).
- `config.json` gains `WifiAPBaked bool` (json `wifiAPBaked,omitempty`) —
  mirrors cloudflared's `Baked`/`Config` split exactly: config.json carries
  only "is it baked", never the SSID/passphrase.
- **v1 scope: arm64 only** (epic decision 5) — `validateWifiAP` mirrors
  `validateIngress`/`validateUsbGadget`; unsupported-board error names which
  selected boards DO support it.
- Collision check vs `--with-external` dests, mirroring `ingress.go`'s
  pattern for `/bin/cloudflared`.
- Config tree: `config/wifi-ap/ssid` and `config/wifi-ap/passphrase`,
  structurally identical to `config/wifi/{ssid,passphrase}` — own
  `.explain.md` sidecars, empty-means-unset. Empty SSID = feature inert even
  if baked (mirrors `wifiup.Run`'s "no credentials, do nothing"). Empty
  passphrase = open AP (stays inside the existing WPA2-PSK/open-only WiFi
  scope lock — no WPA3/EAP).
- Secret handling: extend `cmd/gosd-init/internal/boot/sequence.go`'s
  existing WiFi passphrase redaction rule (feeds `internal/redact` for crash
  reports) with a parallel entry for `config/wifi-ap/passphrase`. SSID stays
  unredacted (broadcast on purpose), same as the STA side. Do not invent a
  new redaction mechanism.
- `gosd run` also gains `--wifi-ap` (mirrors how `--ingress` was added to
  both `build.go` and `run.go`) so qemu-virt (arm64) exercises the config-
  plumbing path in CI — real AP association is bench-only (bean 6), but the
  "baked but not configured" / flag-plumbing behavior should get the same
  qemu smoke coverage `--ingress cloudflared` got in bean gosd-66ax.
- Fixture-driven integration test per CLAUDE.md's own convention for
  `gosd build` behavior (network-tripwire RoundTripper, reads the built
  image back) — pattern in `cmd/gosd/build_integration_test.go` and
  `cmd/gosd/ingress_integration_test.go`.

## Todos

- [ ] `internal/wifiappin` (or fold into a shared artifact-pin package):
      pin the hostapd artifact per bean 1's shape
- [ ] `cmd/gosd/wifiap.go`: flag + parse/validate/collision, actionable
      errors, unit tests
- [ ] `internal/initcfg.Config.WifiAPBaked` + pipeline wiring
- [ ] Config tree entries `config/wifi-ap/{ssid,passphrase}` + `.explain.md`
- [ ] Redaction-rule extension in `boot/sequence.go`
- [ ] `gosd run --wifi-ap`
- [ ] Fixture-driven build integration test (tripwire + baked/opt-out cases)
