---
# gosd-eptf
title: Docs, example, and COMPATIBILITY for app env vars
status: completed
type: task
priority: normal
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-09T10:47:58Z
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
- [x] runtime.md + security note
- [x] examples/hello demo + docs pointers
- [x] COMPATIBILITY row

## Summary of Changes

- docs/runtime.md: new "App environment variables (gosd.toml [env])" section, right after the existing GOSD_BOARD/GOSD_HOSTNAME/GOSD_DATA "Environment variables" section — covers os.Getenv consumption, the two sources (gosd.toml [env] / baked --env) and per-key precedence, the clean-slate (no os.Environ inheritance) note, the reserved GOSD_* rule, missing/empty-is-fine, quote-your-values coercion behaviour, and the plaintext-on-the-card security note.
- examples/hello: reads an optional GREETING env var (stdlib os.Getenv only) and appends it to the startup log and HTTP response when set; unset behaviour is byte-for-byte unchanged. Verified it cross-compiles for both linux/arm64 and linux/arm GOARM=6.
- docs/publishing.md: new short "Baking default app environment variables" subsection pointing developers at --env and the gosd.toml [env] override.
- docs/flashing.md: brief, jargon-free "Extra settings" note pointing end users at the gosd.toml section for apps that need it.
- COMPATIBILITY.md: added the "App env vars (gosd.toml [env])" row, ✅ across all four boards, relying on the existing fleet-wide no-hardware-bring-up banner rather than a new footnote (board-agnostic, code-only feature).
