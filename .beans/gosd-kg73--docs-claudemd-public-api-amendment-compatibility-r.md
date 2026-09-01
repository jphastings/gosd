---
# gosd-kg73
title: Docs + CLAUDE.md public-API amendment + COMPATIBILITY row for wifi.Join
status: todo
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:28:29Z
parent: gosd-ojbm
blocked_by:
    - gosd-uy4x
---

Part of the runtime-WiFi-join epic. Depends on the wifiup-reconciler bean being on main (documents shipped behavior, not plans).

- New doc for the app-facing API (follow how `fault/` is documented by docs/crash-reports.md — a focused doc, linked with a descriptive phrase, never a bare path; the repocheck ratchet enforces this for new prose). Cover: wifi.Join's contract, per-call persistence and what it means across reboot/reflash (configstore), the honest failure-reason caveat (wrong passphrase ≈ handshake timeout), off-device behavior, boards without WiFi, and the AP-mode interaction note (epic decision 9).
- docs/ingress.md: the "fix a credential → reboot" paragraph gains the distinction that a *network* change no longer needs a reboot (automatic restart on runtime join), while credential fixes still do.
- CLAUDE.md: add `wifi/` to the public-API-surface bullet (mirror how fault/ is described) and a one-line locked-decision pointer to the epic.
- COMPATIBILITY.md: add a feature row for runtime WiFi join (the board × feature table is hand-maintained — remember it yourself, repocheck won't).
- docs/runtime.md: mention wifi.Join wherever the app-facing runtime contract enumerates capabilities, if it does.

Branch `bean/gosd-kg73-wifi-join-docs` from main; `no release notes` label (the wiring bean carried the release note). Quality gates as usual; do not merge — JP reviews.
