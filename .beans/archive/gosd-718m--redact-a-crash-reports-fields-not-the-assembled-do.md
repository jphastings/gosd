---
# gosd-718m
title: Redact a crash report's fields, not the assembled document
status: scrapped
type: task
priority: normal
created_at: 2026-08-20T05:37:13Z
updated_at: 2026-08-20T12:18:28Z
parent: gosd-m6py
---

Deferred from `gosd-15ld`. **Superseded before it was started** — see
"Reasons for Scrapping". The description below is the code as it stood when
this was filed, on 2026-08-20; `main` no longer looks like it.

`faultreport.Render` assembles the whole body — gosd's own constant prose, its
headings, and `detailText`'s indented Technical detail block — and only then
runs `redact.Redact` over the result. Two consequences:

1. `detailText`'s documented "no content can break out of it" property is
   upheld only by `redact` refusing to place a control character (added by
   `gosd-15ld`), not structurally. Redacting Doing/Problem/Fix/Detail/AppName
   BEFORE `body()` assembles and `detailText` indents would restore it as a
   property of the shape rather than of a collaborator's discipline.
2. Redaction currently rewrites gosd's own boilerplate, which can never contain
   a secret and so can only be damaged by a collision — the subject of the
   sibling boilerplate-collision bean. Per-field redaction fixes both at once,
   which is why they belong together rather than split across two changes.

Not urgent: with replacements sanitised there is no injection left to close.
This is the structural cleanup.

## Todos

- [x] Move redaction to per-field, before `body()` assembly and `detailText` indentation — shipped by `gosd-ywsv`
- [x] Decide what happens to `Result.Skipped` accounting when rules are applied per field — answered by `Redactor.Skipped`
- [x] Re-word `detailText`'s docstring once the property is structural again

## Reasons for Scrapping

**Already built, by `gosd-ywsv` in PR #340, while this bean sat in review on
the `gosd-15ld` branch.** Every line of the proposal is now the shape of the
code on `main`, so there is nothing left to carry forward:

- `redact` no longer offers a one-shot `Redact(body, rules)` at all. It is
  `New(rules) Redactor` plus `Redactor.Apply(text)`, prepared once and run
  over as many strings as the caller has — exactly the shape per-field
  redaction needs.
- `faultreport.Render` calls a new `scrub` **before** `frontmatter` or
  `body` assembles anything, redacting `Code`, `Doing`, `Problem`, `Fix`,
  `Detail`, `AppName`, `AppVersion` and `SupportURL` as individual values.
  `detailText` now indents text that was already redacted, which is item 1
  of this bean.
- gosd's own boilerplate is no longer redacted, which is item 2 (and the
  sibling boilerplate-collision bean, `gosd-fu1z`, also in #340).
- The `Result.Skipped` question is answered by `Redactor.Skipped()`:
  skipping is decided once per rule set in `New`, not once per string, so
  one answer covers every field.

Sanitising the replacement text itself was never this bean's — that half
stayed with `gosd-15ld`, and lands separately at `redact.New`.
