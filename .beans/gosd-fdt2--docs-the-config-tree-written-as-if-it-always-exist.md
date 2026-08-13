---
# gosd-fdt2
title: 'Docs: the config tree, written as if it always existed'
status: todo
type: task
priority: normal
created_at: 2026-08-13T15:40:14Z
updated_at: 2026-08-13T15:40:23Z
parent: gosd-rw6n
blocked_by:
    - gosd-87ip
---

Documentation for epic gosd-rw6n. Per JP: **no mention of the previous format
anywhere** — every page is written as if the config tree always existed. No
"formerly gosd.toml", no migration notes, no legacy sections.

## Todos

- [ ] New `docs/config.md` (replaces `docs/gosd.toml.md`, which is deleted):
      the tree layout, editing values on a mounted card, explain.md files,
      reserved names and the FAT-junk rules, padding/reservation, how the
      Imager wizard's values land in the tree, the store/restore behaviour
      (`.new`, `.unused`, revert-is-a-default), and the secrets note
      (plaintext on the boot FAT; a copy lives in /data via the store, so a
      reflash is not a credentials wipe — only clearing /data is)
- [ ] `docs/image-injection.md`: the manifest `config` array and the TS
      `config` option; the placeholder contract unchanged
- [ ] `docs/runtime.md`: the app-environment section and the
      persistence/reflash section rewritten around the tree and the store
- [ ] Sweep every other reference: `docs/ingress.md`, `docs/publishing.md`,
      `docs/flashing.md`, `docs/provisioning-formats.md`, README,
      COMPATIBILITY.md
- [ ] `docs/releases/UNRELEASED.md`: the standing call-out rewritten to
      describe the tree only (the current text describes `--env-placeholder`,
      which never shipped)
- [ ] CLAUDE.md: the "Naming surfaces" and provisioning locked decisions
      updated (gosd.toml -> the config tree), and the epic recorded
- [ ] npm README already updated in the build-side bean; verify no stale
      references remain in `js/`
