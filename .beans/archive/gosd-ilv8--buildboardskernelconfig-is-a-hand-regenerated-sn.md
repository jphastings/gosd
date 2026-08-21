---
# gosd-ilv8
title: build/boards/*/kernel.config is a hand-regenerated snapshot with nothing verifying it matches the real build
status: completed
type: task
priority: low
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T06:07:12Z
---

**Severity: Low.** Purely a trust/process gap — the file is documented as
advisory, not authoritative — but CLAUDE.md itself already records one
incident (`gosd-95yu`) where reading a stale snapshot like this instead of
the real fragment or a published artifact fed a wrong fact all the way to a
release note.

## Verified

Each board's `build/boards/<board>/README.md` documents `kernel.config` as a
committed-for-reference snapshot, e.g. `build/boards/pi-3b/README.md:27-30`:

> `kernel.config` — the full `.config` a real build produces, committed for
> reference and cross-build diffing (bean gosd-0nl7...). **Generated, never
> hand-edited; regenerate via `gosd build-kernel --board pi-3b -o out/` and
> copy `out/kernel.config` here.**

Nothing in `go test ./...`, CI, or any other automated gate diffs this file
against what `internal/kernelspec.KernelSpec` would actually produce for that
board today. It's regenerated only when a human remembers to, after running
a real (Docker-backed, 20-60 minute) `gosd build-kernel`. CLAUDE.md's own
"board work" section already names the failure mode this invites: a fragment
change lands, the snapshot isn't regenerated, and the stale file sits in the
repo indefinitely looking authoritative to anyone who greps it instead of
the fragment or a published artifact — which is exactly what happened once
already (`gosd-95yu` believed "the Pi family has no ext4" off these
snapshots for months after the fragments actually gained it).

## Fix direction (not locked)

Not proposing a change to the generation workflow itself (regenerating
requires the same 20-60 minute Docker build regardless). Cheaper options:
- A CI job (or a `gosd build-kernel --check` mode) that, when it *does* run
  a build for other reasons, diffs the fresh `kernel.config` against the
  committed one and fails/warns on drift — catching staleness opportunistically
  rather than gating every fragment change on a full rebuild.
- At minimum, strengthen each README's wording to state explicitly how stale
  the file is allowed to get, and point reviewers at the fragment (or a
  released artifact) as the thing to actually trust for a capability claim —
  CLAUDE.md already says this; the README doesn't yet.

## Todos

- [x] Decided: an automated check IS worth it, and is cheap enough to run in
      every `go test ./...` without needing a real Docker build - it checks
      the committed snapshot against kernelspec's CURRENT
      RequiredY/ForbiddenY/ModulesDisabled assertions (not byte-identity
      with a fresh build, which does need Docker), which is exactly the
      class of staleness that bit gosd-95yu. This does NOT make the
      snapshot authoritative - the fragment stays the assertion; the test
      only checks the snapshot AGAINST the assertion.
- [x] Implemented `TestKernelConfigSnapshotMatchesAssertions` (new file
      internal/kernelspec/kernelconfigsnapshot_test.go): for every
      kernel-building board, every RequiredY entry must be =y in the
      committed kernel.config, every ForbiddenY entry must not be =y/=m,
      and CONFIG_MODULES must read "is not set". Running this for real
      immediately found six boards' snapshots ALREADY stale (see Summary) -
      exactly the failure mode this bean predicted. Since fixing those
      needs real Docker builds (out of scope here, see CLAUDE.md), the
      known instances are recorded in a `knownKernelConfigSnapshotDrift`
      ratchet map (same pattern as kernelspec_test.go's
      `dtbExemptFromArtifacts`): the test fails on any NEW drift beyond
      what's listed, and also fails if a listed entry stops reproducing
      (forcing the map to be kept in sync, so it can only shrink). Bean
      gosd-dcov tracks actually regenerating the six stale snapshots.
