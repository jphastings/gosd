---
# gosd-0jl3
title: 'Build rail: gosd build --wifi-ap flag + config tree + integration test'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T06:40:17Z
parent: gosd-qfbk
blocked_by:
    - gosd-hawd
    - gosd-tufx
---

WiFi AP epic gosd-qfbk bean 4 (needs bean 1's artifact shape + bean 3's
package for field names). Blocks bean 5 (runtime wiring needs the config.json
bit this bean defines).

## Locked decisions

- `gosd build --wifi-ap` — flag shaped like `--ingress cloudflared`: embeds
  the pinned hostapd binary via the existing `pipeline.ExtraExecutables`
  mechanism (the same one `--ingress cloudflared` already uses to land
  `/bin/cloudflared`).
- **Boot-state default is build-configurable** (epic decision 11a): the
  build flag / `gosd.toml` decides whether the AP is enabled or disabled at
  boot. The app can flip it at runtime through the public `wifiap` package
  (bean 3), so this sets the starting state, not a permanent one.
- **SSID defaults to one derived from the device hostname** when not set via
  flag/`gosd.toml`/the config tree (epic decision 11b) — a baked image is
  usable with zero configuration. Reuse the existing hostname sanitization
  rather than writing a second one (see the default-hostname locked decision
  in CLAUDE.md: sanitized basename of the app's main package, overridable via
  `config/hostname`).
- `config.json` gains the baked bit + the default boot state — mirrors
  cloudflared's `Baked`/`Config` split: config.json never carries the
  passphrase.
- **BOTH arm64 and armv6** (epic decision 5) — `validateWifiAP` mirrors
  `validateIngress`/`validateUsbGadget` in shape, but unlike `--ingress`
  there is no board exclusion to enforce unless bean 1 reports the armv6
  cross-compile intractable. If every selected board is supported, the
  validation is mostly a collision/consistency check.
- Collision check vs `--with-external` dests, mirroring `ingress.go`'s
  pattern for `/bin/cloudflared`.
- Config tree: `config/wifi-ap/ssid` and `config/wifi-ap/passphrase`,
  structurally identical to `config/wifi/{ssid,passphrase}` — own
  `.explain.md` sidecars, empty-means-unset. Empty SSID now means "derive
  from hostname" (decision 11b), NOT "inert" — the earlier
  empty-SSID-disables-the-feature proposal was superseded; the boot state is
  what enables/disables. Empty passphrase = open AP (stays inside the
  existing WPA2-PSK/open-only WiFi scope lock — no WPA3/EAP).
- Secret handling: extend `cmd/gosd-init/internal/boot/sequence.go`'s
  existing WiFi passphrase redaction rule (feeds `internal/redact` for crash
  reports) with a parallel entry for `config/wifi-ap/passphrase`. SSID stays
  unredacted (broadcast on purpose), same as the STA side. Do not invent a
  new redaction mechanism.
- `gosd run` also gains `--wifi-ap` (mirrors how `--ingress` was added to
  both `build.go` and `run.go`) so qemu-virt (arm64) exercises the config-
  plumbing path in CI — real AP association is bench-only (bean 6).
- Fixture-driven integration test per CLAUDE.md's own convention for
  `gosd build` behaviour (network-tripwire RoundTripper, reads the built
  image back) — pattern in `cmd/gosd/build_integration_test.go` and
  `cmd/gosd/ingress_integration_test.go`.

## Todos

- [ ] Pin the hostapd artifact per bean 1's shape (both arches)
- [ ] `cmd/gosd/wifiap.go`: flag + boot-state option + parse/validate/
      collision, actionable errors, unit tests
- [ ] Hostname-derived SSID default (reusing existing sanitization)
- [ ] `internal/initcfg.Config`: baked bit + default boot state + pipeline
      wiring
- [ ] Config tree entries `config/wifi-ap/{ssid,passphrase}` + `.explain.md`
- [ ] Redaction-rule extension in `boot/sequence.go`
- [ ] `gosd run --wifi-ap`
- [ ] Fixture-driven build integration test (tripwire + baked/opt-out +
      hostname-derived-SSID cases)
