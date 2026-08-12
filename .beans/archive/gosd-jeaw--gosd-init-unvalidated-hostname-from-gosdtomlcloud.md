---
# gosd-jeaw
title: 'gosd-init: unvalidated hostname from gosd.toml/cloud-init makes sethostname fatal — a hand-edited card reboot-loops the device'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:51:53Z
updated_at: 2026-07-31T07:51:53Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`boot.Run` treats a failed `SetHostname` as fatal (sync + 5s + reboot) at
cmd/gosd-init/internal/boot/sequence.go:255 (gosd.toml re-apply; same for
the earlier config.json apply). The hostname comes verbatim from
`gosdtoml.Parse` or cloud-init user-data — no validation anywhere — and the
real implementation is `unix.Sethostname`, which returns EINVAL for names
over 64 bytes.

**Failure scenario:** a user hand-edits `gosd.toml` to a 65+-char hostname
(the documented fallback flow encourages editing this file). Every boot:
parse → sethostname → EINVAL → fatal → reboot. The file persists, so the
loop is permanent, with no boot-failure.log (only haltForDataCorruption
writes one). This violates the design rule that a malformed provisioning
file must never stop boot (internal/provision's own package doc).

**Fix:** validate/clamp before applying (63-byte cap + charset via
`naming.Sanitize`, which itself has no length cap today — add one), log
rejections and keep the previous hostname; and demote both SetHostname
call sites from fatal to log-and-continue — a wrong hostname is cosmetic,
a reboot loop is not.

Related: the build-side `--hostname` flag is also unvalidated (separate
bean, cmd/gosd area) — fixing both closes the class.

## Summary of Changes

- `internal/naming.Sanitize` now caps its output at a new `naming.MaxLength`
  (63 bytes, sethostname(2)'s usable limit), re-trimming any hyphen the cap
  exposes at the new end. Existing callers (`gosd build`'s default hostname,
  `--hostname` sanitization) get the cap for free.
- `cmd/gosd-init/internal/boot/sequence.go`: added `validHostname`, which
  accepts a candidate hostname only if it's already in `naming.Sanitize`'s
  canonical form (`name == naming.Sanitize(name)`) — this single check
  covers both the charset and the length cap. Both places a provisioning
  source can override the hostname (cloud-init user-data, gosd.toml) now run
  the candidate through `validHostname` before assigning it to `cfg.Hostname`;
  an invalid candidate is logged and the previous hostname is kept instead of
  being silently mangled to fit.
- All three `SetHostname` call sites (the initial config.json apply, and the
  gosd.toml/cloud-init re-applies) now go through a new `applyHostname`
  helper that logs a `SetHostname` failure and lets boot continue, instead of
  calling `fatal()` (sync + 5s + reboot). A wrong hostname is cosmetic; a
  hostname that can never be applied (e.g. a value that slips past
  validation, or a kernel-level restriction) must not turn into a permanent
  reboot loop.
- Tests: `internal/naming/naming_test.go` covers the length cap and the
  hyphen-at-truncation-boundary edge case.
  `cmd/gosd-init/internal/boot/sequence_test.go` covers: an over-long
  gosd.toml hostname is rejected and the previous hostname kept (both
  SetHostname calls); an invalid-charset cloud-init hostname likewise; and a
  SetHostname failure no longer reboots (replaces the old
  `TestRunFatalPathOnHostnameFailure`, since that path is no longer fatal).
