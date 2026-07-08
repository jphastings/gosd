---
# gosd-xnbd
title: 'gosdtoml: [env] schema, parser coercion, and template section'
status: todo
type: task
created_at: 2026-07-08T03:28:43Z
updated_at: 2026-07-08T03:28:43Z
parent: gosd-9b5c
---

Foundational; blocks the injection and builder tasks.
- Add `Env map[string]string` to internal/gosdtoml Config (`toml:"env"`).
- Parse coercion per the locked rules: quoted strings pass through; bare scalar (int/float/bool) → its string form + a returned warning; array/table/datetime under [env] → skip that key + warning. The parser must surface warnings to the caller (extend the existing return shape or add a warnings slice) so gosd-init can log them; malformed [env] never fails the whole parse (whole-file-optional invariant holds).
- template.go: add an [env] section to the generated gosd.toml with a plain-language comment header (match the existing hostname/wifi header voice — for a semi-technical reader: "Extra settings your app reads. Add lines like NAME = \"value\"."). If baked env values exist, render them live; else show a commented example. 
- Tests: parse (strings, coercion+warning, skipped non-scalars, empty/missing), and exact-output render (with and without baked values).
- [ ] Env field + coercion + warnings plumbed
- [ ] Template [env] section + header
- [ ] Parse + render tests
