---
# gosd-u2gz
title: 'provsnapshot: classify [ingress.tailscale-funnel] (snapshot whole, like WiFi)'
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:09:44Z
updated_at: 2026-08-08T04:49:46Z
parent: gosd-65uy
blocked_by:
    - gosd-85bn
    - gosd-tgzo
---

Tailscale epic gosd-65uy bean 6 (after TS-1 gosd-85bn; cloudflared's
gosd-tgzo lands the table-driven [ingress.<agent>] classification this bean
extends).

## Locked decisions

- [ingress.tailscale-funnel] snapshots WHOLE-SECTION, like WiFi and
  [ingress.cloudflared] — one new table row + tests, no new classification
  logic. Authkey = operator intent; baked default is always empty in v1, so
  any present section is a hand-edit (= fresh intent).
- Test the LAYERED reflash property explicitly: the toml section is restored
  by the snapshot AND the node identity already lives on /data
  (/data/.gosd/tailscale, epic decision 3) → after a plain Imager reflash of
  a --data-size=expand image the device reconnects as the SAME node with the
  SAME public URL and no re-auth. Assert both layers.

## Todos

[x] classification table row + tests
[x] layered reflash-property test


## Summary of Changes

- `cmd/gosd-init/internal/provsnapshot/provsnapshot.go`: added one row to
  `ingressSections` for `TailscaleFunnel` (`configured` checks
  `ing.TailscaleFunnel.Configured()`, `restore` copies the whole
  `TailscaleFunnel` section) — no other logic changed, since `planRestore`,
  `apply` and `effective` already loop over the table (gosd-wxjy). Extended
  the package doc with a paragraph on `[ingress.tailscale-funnel]`: same
  whole-section classification as `[ingress.cloudflared]`, plus the epic's
  authkey/state nuance — the authkey is only needed for first registration
  and is safe to delete afterwards (epic decision 4), because the node's
  real identity lives on `/data/.gosd/tailscale` (epic decision 3), entirely
  outside this package's reach, so it survives a reflash on its own.
- `cmd/gosd-init/internal/provsnapshot/provsnapshot_test.go`: four new
  tests mirroring gosd-tgzo's cloudflared coverage —
  `TestSnapshotRecordsAHandSetTailscaleFunnelSection`,
  `TestTailscaleFunnelSurvivesAPlainReflashWithNoCredentialsFile`,
  `TestReflashKeepsAHandSetTailscaleFunnelSectionMadeOnTheNewCard`, and the
  bean's requested layered-property test,
  `TestTailscaleFunnelReflashRestoresSettingsEvenAfterTheAuthkeyWasRemoved`:
  it seeds a snapshot whose Funnel section already has no authkey (as if the
  operator removed it after first registration) and asserts the restore
  still fires and still writes hostname/port/funnel_port back to gosd.toml,
  with a doc comment spelling out that the matching identity/URL layer is
  provably out of this package's reach (it lives on /data, per epic decision
  3) and is only asserted in that comment and the epic bean, not in code
  here.
- Crash-ordering: unchanged. This bean only adds a table row and reuses the
  existing digest-last `gosd.toml` → `snapshot.json` write order; no new
  on-disk artifact or commit point was introduced.
- Gates: `go test ./cmd/gosd-init/...`, `go test ./...`, `go vet ./...`,
  `gofmt -l .`, `golangci-lint run --allow-parallel-runners ./...`,
  `GOOS=linux golangci-lint run --allow-parallel-runners ./...` all green
  (an isolated `GOCACHE` and one retry were needed for `go test ./...`:
  concurrent activity on the shared machine caused transient stdlib
  "not in std" and ENOSPC failures unrelated to this change, per repo
  CLAUDE.md's guidance on shared-build-cache/disk contention).
