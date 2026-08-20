---
# gosd-fu1z
title: Crash-report redaction rewrites gosd's own boilerplate, which can never contain a secret
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:09:10Z
updated_at: 2026-08-20T05:02:12Z
parent: gosd-m6py
---

**Severity: Low.** Cosmetic, but it degrades precisely the human-facing
prose a non-technical device owner is meant to read, and it is guaranteed to
happen for some ordinary env values.

## What happens

`faultreport.Render` (internal/faultreport/faultreport.go:168) applies
redaction to the ENTIRE assembled document, including gosd's own fixed
strings in `body()` — the title, the "Your device stopped" sentence, the
"read it on any computer" explanation, the "send them this whole file"
instruction, and the no-fix fallback text.

Those strings are compile-time constants in this repo. They cannot contain
an app's secret, so redacting them can never protect anything — it can only
damage the report.

Verified: with an app env of `APPNAME=weatherbox` and the image's baked app
name also `weatherbox`, the rendered report reads:

```
# {$APPNAME} crash report

Your {$APPNAME} device stopped.
```

`computer` is exactly `redact.MinNeedleLength` (8) bytes and appears in the
boilerplate, so any env var whose value is `computer` rewrites the sentence
that explains what the file is. Other 8+ byte collisions with the fixed
prose exist (`whatever`, `specific`, `summary` at 7 is below the floor).

The `MinNeedleLength` floor was chosen to stop short values shredding the
report (internal/redact/redact.go:19-30), but the floor is a heuristic
against a problem that partly disappears if redaction simply never touches
text that cannot hold a secret.

## Fix

Redact per-field rather than per-document: apply the rules to
`Report.Doing`, `Problem`, `Fix`, `Detail` and `Context.AppName` before
`body()` assembles them, leaving gosd's own constants alone. This is the
same change bean gosd-15ld needs to restore `detailText`'s
"cannot break out of the code block" property, and the same change bean
gosd-ywsv needs for `error_code` — one refactor closes all three.

Note `AppName` must still be redacted (it is baked from config.json and an
app could plausibly share a value with an env var), so the rule is "redact
the variable fields", not "redact only app-supplied fields".

## Todos

- [x] Move redaction from the assembled document to the individual variable fields
- [x] Test: an env value equal to a word in gosd's boilerplate leaves the boilerplate intact
- [x] Re-check whether `MinNeedleLength`'s rationale still needs to be as strong once boilerplate is excluded (do not change the value in this bean; just record the finding)

## Summary of Changes

Fixed together with gosd-ywsv and gosd-tzd1 in one PR — see gosd-ywsv for the
single rule the three of them settle on.

- Redaction moved off the assembled document and onto the individual
  variable fields (`faultreport.scrub`), so gosd's own compile-time prose —
  the title, "Your device stopped", the "read it on any computer"
  explanation, the "send them this whole file" instruction, the no-fix
  fallback — is never rewritten. `Context.AppName` stays redacted, as this
  bean specified: an app can plausibly share a value with an env var, so
  the rule is "the variable fields", not "the app-supplied ones".
- `Context.AppVersion` and `SupportURL` joined the redacted set for the
  same reason: they are the developer's own strings, and after this change
  nothing else would have covered them.
- Test: `TestRenderLeavesGosdsOwnWordsAlone` pins both halves — with a rule
  for `computer`, "read it on any computer." survives intact while the same
  word in the app's own `Problem` is replaced. The `secrets-redacted`
  golden carries the same rule, so the boilerplate's survival is visible in
  a fixture too.
- Finding recorded (value unchanged, as instructed): `MinNeedleLength`'s
  doc now says the floor is a little blunter than it needs to be, because
  the worst collisions it was chosen to prevent were with fixed prose that
  redaction no longer touches — and that lowering it is still its own
  decision with its own evidence, since a short value colliding with a
  stack trace's text is untouched by that argument.

Worth knowing for gosd-15ld: because `Detail` is now redacted before
`detailText` indents it, a replacement carrying its own line breaks is
indented with the rest of the code block rather than landing at column 0
inside it. That is half of gosd-15ld; sanitising the replacement text
itself, which still reaches gosd's prose through `Doing`/`Problem`/`Fix`,
remains that bean's work and is called out in `scrub`'s doc comment.
