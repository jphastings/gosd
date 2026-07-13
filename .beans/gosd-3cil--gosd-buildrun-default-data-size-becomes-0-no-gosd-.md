---
# gosd-3cil
title: 'gosd build/run: default --data-size becomes 0 (no GOSD-DATA partition)'
status: completed
type: task
priority: normal
created_at: 2026-07-13T06:00:25Z
updated_at: 2026-07-13T06:18:19Z
---

JP, 2026-07-13: the default image should carry NO writable data partition — `--data-size 0` semantics — instead of today's 1GiB. Persistence becomes strictly opt-in (`--data-size 1GiB` etc.). Rationale: appliance images shouldn't pay a 1GiB image-size/flash-time cost unless the app actually wants /data.

## Locked decisions

- `defaultDataSize` (cmd/gosd/build.go, also used by `gosd run`) changes "1GiB" → "0". Explicit `--data-size` behavior unchanged.
- Everything downstream already supports this: `internal/image` omits partition 2 at zero (single-partition layout), and gosd-init boots fine without GOSD-DATA ([[gosd-hcca]]'s read-only fallback — a missing partition is never fatal). No runtime code changes expected; if any turn out to be needed, stop and report.
- Flag help text must state the new default and how to opt in; docs/runtime.md's /data section updated (currently says "1GiB unless you say otherwise"); COMPATIBILITY.md's persistent-/data row gets its footnote updated to say the partition is opt-in at build time (status stays ✅ — the capability is unchanged).
- Tests: update anything pinning the old default (build integration tests asserting partition layout); add/keep coverage for BOTH shapes — default → single partition, explicit `--data-size 512MiB` → two partitions with GOSD-DATA.
- CI's qemu boot-to-HTTP job builds without `--data-size` and will now exercise the no-/data boot path — that passing is part of the verification, not a problem to work around.

## Todos

- [x] Flip defaultDataSize + help text (build + run share it)
- [x] Tests: default=single-partition, explicit=two-partition
- [x] docs/runtime.md + COMPATIBILITY.md footnote
- [x] Quality gates green (incl. GOOS=linux golangci-lint)

## Summary of Changes

- `cmd/gosd/build.go`: `defaultDataSize` flipped `"1GiB"` → `"0"`; updated its
  doc comment and the `--data-size` flag help text to state the new
  opt-in-by-default behavior. `gosd run` picks this up automatically since it
  shares the same const.
- `cmd/gosd/build_integration_test.go`: the main acceptance test now asserts
  the single-partition layout (no GOSD-DATA) when `--data-size` is omitted;
  added `TestBuildWithExplicitDataSizeAddsTheDataPartition` covering the
  opt-in path (`--data-size 512MiB` → two partitions, GOSD-DATA present); kept
  the explicit `--data-size=0` opt-out test since it's still valid, if now
  redundant with the default.
- `docs/runtime.md`: the "Persistent storage: /data" section now describes
  the partition as opt-in with a `0` default, instead of "1GiB unless you say
  otherwise".
- `COMPATIBILITY.md`: added a `[^data-opt-in]` footnote to the "Persistent
  /data partition" row across all four boards, explaining the partition is
  opt-in at build time; capability status stays ✅ (unchanged).
- No runtime (gosd-init) code changes were needed — the downstream
  single-partition / read-only-fallback handling already covered
  `--data-size 0`.
