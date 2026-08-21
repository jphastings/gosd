---
# gosd-15ld
title: 'Crash-report Markdown injection: redaction Replacement is trusted but comes from the SD card'
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:08:24Z
updated_at: 2026-08-20T05:38:04Z
parent: gosd-m6py
---

**Re-validated 2026-08-20.** The trust boundary is intact and relocated,
exactly as suspected. `gosd.toml` is gone (epic `gosd-rw6n`), so the filed
reproduction — a TOML quoted `[env]` key carrying newline escapes — is dead.
`redact.Rule.Replacement` is still trusted, still built from attacker-shaped
input, and still applied to the **assembled** report body.

## The hole, as it stands today

`redact.Redact` (`internal/redact/redact.go`) treats `Rule.Needle` as
untrusted and `Rule.Replacement` as trusted. Both producers derive one from
data outside gosd's control:

- `boot.envRedactionRules` (`cmd/gosd-init/internal/boot/sequence.go`) builds
  `"{$" + key + "}"`, where `key` is now a **file name in `config/env/` on
  the FAT boot partition** — chosen by whoever holds the card. The build
  validates those names; a file created on the card afterwards never went
  through the build.
- `secretreg.Label` builds `"{secret: " + replacement + "}"` from
  `fault.RegisterSecretString`'s second argument. `secretreg` bounds the
  file's size and entry count but never looks at what a replacement
  contains.

**Amended 2026-08-20, after `gosd-ywsv` (PR #340) landed on `main`.** That
work moved redaction per-field: `faultreport.Render` now scrubs `Code`,
`Doing`, `Problem`, `Fix`, `Detail` and the image line as individual values
*before* `frontmatter` or `body` assembles anything, and `redact` exposes
`New`/`Apply`/`Skipped` rather than the one-shot `Redact` this bean was
filed against. That closes the ordering half of the hole — a replacement's
line breaks now get `detailText`'s indent along with everything else — and
it removes gosd's own boilerplate from the redacted set entirely.

It does **not** close the trust boundary. `Rule.Replacement` is still taken
on trust, and `Doing`, `Problem` and `Fix` are prose where no indentation is
coming: a replacement carrying a newline still lands text at column 0 of a
report's own sentences, and one long enough to be content still reflows the
paragraph it substitutes into. `Redactor.Skipped` carries replacements too
and is logged to the console, so it reaches a reader by the same route.
Sanitising the replacement itself is what is left, and it is this bean's.

## What changed about reachability

The card route now needs a name a FAT volume can hold: macOS, Windows and
Linux all refuse control characters in a file name, so plain newline
injection via `config/env/` needs a hand-crafted FAT image (`dd` of one is
well within reach of anyone who can write the card) rather than a file
manager. Two routes need no such assumption:

- **`fault.RegisterSecretString(secret, label)`** takes any string. An app
  that interpolates anything into a label breaks the report's structure.
- **The config store on `/data`.** `configstore.load` reads setting names
  off the data partition, which is ext4 when built with
  `--data-filesystem=ext4`, where a file name may hold any byte but `/` and
  NUL. `restorePlan`'s `default` branch restores an `env/...` entry that is
  neither baked nor on the card, and `restore` calls `config.Set` for it
  **before** any card write is attempted — so the name reaches
  `envRedactionRules` whether or not the FAT card would accept it.

## Fix

Sanitise at the one choke point both producers pass through — `redact.New`,
where a rule set is prepared — rather than at each producer:

- Every `Replacement`, applied or skipped, has control characters
  (`< 0x20`, `0x7f`) removed.
- One longer than `MaxReplacementBytes` (64) — or empty after stripping — is
  replaced outright with `FallbackReplacement` (`{redacted}`), never
  truncated: half a label reads as a name, and the wrong one. The needle is
  still redacted either way, which is the part that matters.

64 is derived rather than picked: the report's prose is hand-wrapped to a
narrow terminal, so a label that fits inside one of those lines can only
substitute a value in-line, where a longer one reflows the paragraph it
landed in. Every honest label — `"{$"` plus a POSIX env name plus `"}"`,
`"{secret: }"` around a word or two — is far shorter.

The env route is now closed twice over: `mergeUserEnv` also drops a card env
name the build would have refused (bean `gosd-q4v5`, same PR), so no such
key reaches `envRedactionRules` in the first place.

## Deliberately not done

**Nothing further.** The per-field redaction this bean deferred to
`gosd-718m` arrived first, from `gosd-ywsv` in PR #340, so `gosd-718m` is
scrapped as superseded and the two halves of the hole are closed by two
separate changes rather than one. `faultreport`'s `scrub` docstring, which
named the replacement half as still outstanding, now names `redact` as
what supplies it.

## Todos

- [x] Re-validate the trust boundary against the config tree
- [x] Sanitise `Rule.Replacement` (control chars stripped, over-long replaced) in `redact.New`
- [x] Sanitise `Redactor.Skipped` the same way
- [x] Test: a replacement carrying a newline cannot place a character at column 0
- [x] Test: an over-long replacement becomes the fallback rather than a truncated name
- [x] Correct/qualify `detailText`'s and `scrub`'s docstrings
- [x] Reject the input at the door too: a card env name the build would refuse is dropped (gosd-q4v5, shipped on `main` by gosd-39da)
- [x] Move redaction to per-field, before `body()` assembly — shipped on `main` by `gosd-ywsv` (PR #340); `gosd-718m` scrapped as superseded

## Summary of Changes

`redact.New` now enforces the shape `Rule` has always documented instead of
trusting it: control characters stripped from every replacement, and an
empty or over-long one swapped for `{redacted}` rather than truncated. It
sits in `New` so it covers both producers, every string a `Redactor` is
applied to, and `Redactor.Skipped`, which is logged to the console.

Rebased onto `main` after `gosd-ywsv` (PR #340) shipped per-field
redaction, which supplied the other half of this bean and rewrote the API
this fix was originally written against. `faultreport.scrub`'s docstring
named the replacement half as outstanding; it now names `redact` as what
supplies it.
