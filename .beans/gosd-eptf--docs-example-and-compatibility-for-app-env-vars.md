---
# gosd-eptf
title: Docs, example, and COMPATIBILITY for app env vars
status: todo
type: task
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-08T03:28:43Z
parent: gosd-9b5c
blocked_by:
    - gosd-r0be
    - gosd-yejj
---

Close the feature out.
- docs/runtime.md: an "Environment variables" section — the app reads config via os.Getenv; sources + per-key precedence (gosd.toml [env] > baked --env); reserved GOSD_* list; the plaintext-on-the-card security note; that missing/empty is fine.
- examples/hello: read and print one optional env var (e.g. GREETING or LOG_LEVEL) to show the pattern, stdlib-only, no-op when unset.
- docs/publishing.md and docs/flashing.md: one line each pointing developers/users at the [env] section as appropriate (flashing.md stays non-technical — frame as "extra settings some apps need").
- COMPATIBILITY.md: add an "App env vars (gosd.toml [env])" row, ✅ for all four boards (board-agnostic, code-only), per the same-PR convention.
- [ ] runtime.md + security note
- [ ] examples/hello demo + docs pointers
- [ ] COMPATIBILITY row
