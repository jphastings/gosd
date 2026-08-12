---
# gosd-tgzo
title: 'provsnapshot: classify [ingress.cloudflared] (snapshot whole, like WiFi)'
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:52:39Z
updated_at: 2026-08-07T17:10:48Z
parent: gosd-virc
blocked_by:
    - gosd-7upw
---

Ingress epic gosd-virc bean 5 (needs gosd-7upw). Classify the new section under
provsnapshot's three-way fresh-intent / snapshot-intent / baked-default test
(package doc L1-105).

## Locked decisions

- Snapshot it whole-section, like the WiFi ssid/passphrase pair — never
  field-merged. Pure operator intent; baked default is always empty in v1, so
  any present section is a hand-edit (= fresh intent).
- The token round-trips through /data exactly as WiFi passphrases do. With no
  credentials file on GOSD-BOOT (epic decision 3), ingress fully survives a
  plain Imager reflash — this is a designed property, assert it in tests.


## Todos

- [x] Add `Ingress gosdtoml.IngressCloudflared` to `Provisioning`, compared whole (never field-merged)
- [x] Classify fresh/snapshot intent per the package doc's three-way test (baked default always empty in v1, so any present section is a hand-edit)
- [x] Extend `planRestore`/`apply`/`effective`/`encode`/`decode` to restore and round-trip the whole section
- [x] Behavioral test asserting the reflash-survival property (no credentials file; token round-trips through /data exactly like a WiFi passphrase)
- [x] Confirm the snapshot's digest-last crash-ordering mechanism still holds (fields added, mechanism unchanged)
- [x] Quality gates + PR

## Summary of Changes

- `cmd/gosd-init/internal/provsnapshot/provsnapshot.go`: added `Ingress gosdtoml.IngressCloudflared` to `Provisioning` (compared via `==` in `equal`, exactly like `Wifi`). Added `freshIngress`: fresh intent is simply "the card's own `[ingress.cloudflared]` is non-empty" — config.json never bakes a real token (only whether the cloudflared binary itself is baked in), so unlike hostname/WiFi there is no baked value to differ from. `planRestore` now restores the whole snapshot `Ingress` value iff there's no fresh intent and the snapshot's own section is non-empty (snapshot intent). Added `plan.Ingress *gosdtoml.IngressCloudflared`; `apply()` merges/blank-guards it the same way as `Wifi`, seeded from the card's own `cfg.Ingress` so an unrelated hostname/WiFi/[env] restore (or vice versa) never blanks it. `effective()` now folds the card's `[ingress.cloudflared]` into what a boot settled on (no cloud-init source to check first — the Imager wizard has no concept of a tunnel). `encode()`/`decode()` round-trip `Effective.Ingress` through the snapshot's own gosd.toml, token included, at the same trust level the WiFi passphrase already has there. `bakedDefault` (snapshot.json's schema) deliberately gained no Ingress field, since config.json can never bake one — there is nothing real to record. Updated the package doc's three-way-test section and the two comments #208 left behind noting ingress restore was not yet implemented.
- `cmd/gosd-init/internal/provsnapshot/provsnapshot_test.go`: added `TestSnapshotRecordsAHandSetIngressSection`, `TestReflashKeepsAHandSetIngressMadeOnTheNewCard` (fresh intent on the new card wins), and `TestIngressSurvivesAPlainReflashWithNoCredentialsFile` — the bean's requested behavioral test, simulating a hand-edited tunnel surviving a reflash purely via the /data snapshot, with no credentials-file API anywhere in `Deps` for it to depend on.
- Crash-ordering check (asked for explicitly): the digest-last commit mechanism is unchanged — `gosd.toml` is still written before `snapshot.json` (which carries `gosd.toml`'s SHA-256), and `decode()` still rejects a mismatch wholesale. This bean only adds fields to the existing `Provisioning`/`Snapshot` structs and to what's rendered into those same two files; no new on-disk artifact and no new commit point were introduced, so the existing write -> sync -> digest-marker ordering covers the new field automatically.
- Note for JP: docs/runtime.md's "What does **not** come back: anything outside hostname/WiFi/[env]" (the reflash-survival explainer) becomes stale now that ingress also comes back. Left untouched here since gosd-d1c2 (the epic's own docs bean) already owns runtime.md's ingress pointer and is explicitly blocked on the wiring bean (gosd-66ax), not on this one — flagging so it isn't missed when d1c2 lands.
- Gates: `go test ./cmd/gosd-init/...`, `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...` all green.
