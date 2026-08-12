# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.4.0 (2026-08-12).

## Breaking changes

(none yet)

## Other call-outs

- **`LAST_FATAL_ERROR.md` no longer embeds a copy of itself (bean
  `gosd-72ga`).** A device's first real-hardware run of the crash-report
  epic surfaced a bug where a `fault.Fatal` report handed to gosd-init also
  echoed its own rendered Markdown to the app's console — which gosd-init's
  crash-tail capture folded straight back into the very report as its own
  technical detail, a thinner, contradictory copy sitting a few lines below
  the real one (`uptime: unknown`, `boot: unknown`, `device: unknown`, even
  though the header just above knew all three). On a device, `fault.Fatal`
  now prints only a short line naming the error code, never the report
  itself; gosd-init logs the complete, real report to the serial console on
  its own once it commits one, which carries detail the app could never
  print for itself. A follow-up bench run found that on the paths that halt
  or reboot immediately, the device could stop before that console copy
  finished transmitting over a slow serial line — the card copy was never
  affected, only what a cable showed — and bean `gosd-fs34` closed that gap:
  every fatal path now blocks until the console has actually drained before
  it halts or reboots. The developer preview `fault.Fatal` prints off
  a device is otherwise unchanged, except that a header field it could never
  honestly answer there (`uptime`, `boot`, `device`) is now left out
  entirely rather than printed as `unknown`, so the preview reads like a
  real report rather than a half-populated one.
