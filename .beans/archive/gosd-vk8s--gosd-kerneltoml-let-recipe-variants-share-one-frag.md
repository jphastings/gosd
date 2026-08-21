---
# gosd-vk8s
title: 'gosd-kernel.toml: let recipe variants share one fragment (a list, or an include)'
status: completed
type: task
priority: low
created_at: 2026-07-30T09:45:00Z
updated_at: 2026-08-20T05:39:35Z
---

Found while writing the Rockchip audio recipes (bean `gosd-lrxz`).

A `[kernel.<board>]` entry takes exactly one `fragment`, so two variants of the
same feature for the same board cannot share any part of their Kconfig. The
audio recipes hit this hard: `rock-4se-analog.fragment`,
`rock-4se-hdmi.fragment` and `radxa-zero-3e-hdmi.fragment` differ by roughly a
dozen lines each, and then carry an identical ~130-line deny-list, three times.
The deny-list is generated from the pinned defconfig (each fragment says how to
regenerate it), so the copies are consistent today — but a kernel bump means
editing three files, and a reviewer has to diff them to see they match.

Two obvious shapes, either would do:

- `fragment = ["common.fragment", "hdmi.fragment"]` — accept a list and merge
  in order. Cheap: the overlay already concatenates GoSD's fragment and the
  developer's, so this is one more level of the same thing.
- An `include`/`extends` key on the fragment or the board table.

Prefer the list: no new file format, and the merge order is visible at the call
site. `internal/kernelconfig`'s `Fragment string` becomes a `[]string`-or-string
union (TOML makes that ugly; a separate `fragments` key that is mutually
exclusive with `fragment` may be cleaner), and `internal/kernelbuild`'s overlay
concatenates them before writing `overlay-fragment.config`. The cache key must
hash all of them, in order.

Not urgent — three duplicated deny-lists are ugly, not wrong — but the next
feature with a cheap and an expensive variant will want it too.

## Summary of Changes

Implemented the separate-key shape (mutually exclusive with `fragment`, per
the bean's own preference, since a `[]string`-or-string TOML union is
genuinely ugly to decode strictly):

- `internal/kernelconfig.BoardOverlay` gained `Fragments []string`
  (`toml:"fragments"`) alongside the existing `Fragment string`. `Parse`
  rejects a board section setting both, naming the board.
- `Config.Overlay` resolves whichever of `Fragment`/`Fragments` is set into
  an ordered path list, reads each file, and concatenates them (inserting a
  newline between any pair where the first file is missing its own trailing
  newline, so two files can never glue their content across a line) into
  the single `kernelbuild.Overlay.ConfigFragment` blob. `internal/kernelbuild`
  itself is unchanged: it always receives one already-merged fragment, so
  its cache key (`cache.go`'s `OverlayFragment []byte`) already hashes the
  full ordered concatenation with no changes needed there — satisfying the
  bean's "cache key must hash all of them, in order" requirement as a
  consequence of where the merge happens, not a separate mechanism.
- No glob support on `fragments` (unlike `patches`): merge order must match
  list order exactly, which a sorted glob can't promise.
- Added `internal/kernelconfig` tests: parsing a `fragments` list, the
  fragment/fragments mutual-exclusivity error, list-order merging (a later
  file's option wins a conflict with an earlier one, matching
  `merge_config.sh -m`'s own semantics), the inserted-newline behavior, and
  a missing-file error naming the path.
- Documented the new key in `docs/custom-kernels.md` (reference block plus
  a new "Sharing a fragment between variants" section) and added a change
  file.
- Deliberately did **not** touch `examples/chime`'s existing
  `rock-4se-analog.fragment`/`rock-4se-hdmi.fragment`/
  `radxa-zero-3e-hdmi.fragment` duplication, or any board's baked
  `build/boards/*/kernel.fragment` content — this bean is the recipe-format
  capability only; migrating the example (or any board fragment, which
  would need an artifacts release) is a separate, optional follow-up.
