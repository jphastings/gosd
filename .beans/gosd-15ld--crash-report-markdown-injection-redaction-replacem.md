---
# gosd-15ld
title: 'Crash-report Markdown injection: redaction Replacement is trusted but comes from the SD card'
status: todo
type: bug
created_at: 2026-08-12T04:08:24Z
updated_at: 2026-08-12T04:08:24Z
parent: gosd-m6py
---

**Severity: Medium.** Not RCE, but it is a content-injection primitive that
crosses the exact trust boundary this feature is built around: the report
"invites its reader to forward the whole file" (docs/crash-reports.md:214),
so injected content reaches a third party — the developer or support
engineer — that the attacker otherwise has no channel to.

## The hole

`redact.Redact` treats `Rule.Needle` as untrusted but `Rule.Replacement` as
trusted. Neither producer of a Replacement sanitises it, and both derive it
from data outside gosd's control:

- `envRedactionRules` (cmd/gosd-init/internal/boot/sequence.go:689) builds
  `Replacement: "{$" + key + "}"` where `key` is an `[env]` key from
  **gosd.toml on the FAT boot partition** — editable by anyone holding the
  card, on any computer, with no tools.
- `secretreg.Label` (internal/secretreg/secretreg.go:118) builds
  `"{secret: " + replacement + "}"` from the app's own
  `fault.RegisterSecretString` second argument.

`faultreport.Render` (internal/faultreport/faultreport.go:168) assembles the
whole body — including gosd's own fixed prose and the already-indented
Technical detail code block — and only THEN runs redaction over it. So a
Replacement containing a newline lands at column 0 anywhere the needle
occurred, including inside gosd's own sentences.

This also falsifies the claim on `detailText`
(internal/faultreport/faultreport.go:332-334): *"renders Detail as an
indented code block, which — unlike a fenced one — no content can break out
of."* Content injected by the redaction pass breaks out of it, because
indentation happens before redaction.

## Verified attack, end to end

Confirmed by running the real `gosdtoml.Parse` -> `mergeUserEnv` ->
`envRedactionRules` -> `faultreport.Render` chain. Attacker writes to
gosd.toml on the card:

```toml
[env]
"X\n\n## Your device is compromised\n\n![](https://evil.example/beacon.png)\n" = "computer"
```

TOML quoted keys accept `\n` escapes; `gosdtoml.Parse` accepts this with
**zero warnings** (verified). The value `computer` is chosen because it is
exactly `redact.MinNeedleLength` (8) bytes AND appears in gosd's own
boilerplate line "read it on any computer" — so the substitution fires on
**every** crash report regardless of what the app does. Rendered output:

```
This file was written by the device itself, onto its own SD card, so you can
read it on any {$X

## Your device is compromised

![](https://evil.example/beacon.png)
}. Nothing was sent anywhere.
```

The injected heading sits inside gosd's own paragraph, so it reads in gosd's
voice. What that buys an attacker:

1. A forged "what to do next" section instructing the reader to visit a
   phishing URL or run a command — indistinguishable from gosd's own text.
2. `![](...)` beacons when the file is rendered in a GitHub issue, Slack or a
   support portal, leaking the reader's IP/UA and confirming receipt.
3. Forged content presented as a stack trace by escaping the code block.

## Fix

Sanitise the Replacement at the one choke point both producers pass through
— `redact.Redact` (internal/redact/redact.go:88):

- Before applying rules, map every rune `< 0x20 || == 0x7f` in
  `rule.Replacement` to nothing (or U+FFFD), and cap Replacement length
  (64 bytes is generous for a label).
- Do the same for the `Skipped` slice, which is logged to the console.

Belt and braces, both worth doing:

- **Redact the fields, not the assembled document.** Apply redaction to
  Doing/Problem/Fix/Detail/AppName in `faultreport.Render` BEFORE `body()`
  assembles and `detailText` indents. This restores `detailText`'s documented
  "cannot break out" property structurally, and stops redaction from
  rewriting gosd's own constant prose, which can never contain a secret and
  so can only be damaged (see sibling bean on boilerplate collisions).
- **Reject the input at the door.** `gosdtoml` should drop-with-warning any
  `[env]` key containing a control character. It currently accepts them
  silently, and such a key is not a valid POSIX environment variable name
  anyway.

## Todos

- [ ] Sanitise `Rule.Replacement` (control chars stripped, length capped) in `redact.Redact`
- [ ] Sanitise `Result.Skipped` the same way
- [ ] Move redaction to per-field, before `body()` assembly and `detailText` indentation
- [ ] `gosdtoml`: warn-and-drop `[env]` keys containing control characters or otherwise invalid as env var names
- [ ] Test: hostile gosd.toml `[env]` key cannot place a character at column 0 of the rendered report
- [ ] Test: `RegisterSecretString(secret, "a\nb")` cannot break the code block
- [ ] Correct `detailText`'s docstring if any escape route remains
