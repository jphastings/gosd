---
# gosd-0hkx
title: 'Docs refresh: Diátaxis reshape, de-staling, and density pass across README, COMPATIBILITY, and docs/'
status: completed
type: task
priority: normal
created_at: 2026-07-31T03:53:23Z
updated_at: 2026-07-31T06:36:09Z
---

Reshape, refine, and improve the readability and information density of the user/developer-facing docs (README.md, COMPATIBILITY.md, docs/*.md — excluding docs/design/, which holds bean-shaped design spikes).

Guiding principles (locked for this task):
- Diátaxis framing: each doc serves one primary quadrant (tutorial / how-to / reference / explanation); reshape mixed docs so the quadrants are clearly sectioned rather than interleaved.
- Preserve every technical fact, warning, and hard-won gotcha. Density comes from cutting redundancy and narrative scaffolding ("the finding that changes the plan", session history), not substance.
- Apply the project docs rule: no frequently-changing facts stated as current truth (bring-up status, artifact version numbers, dates) — point at the source of truth instead. The README's pre-release note claiming no hardware boot has happened is stale (COMPATIBILITY.md records four completed bring-ups) and is a concrete instance to fix.
- No file renames (docs paths are referenced from CLI help text, CLAUDE.md, and each other). Heading anchors that are cross-referenced must survive or have every referrer updated in the same PR.
- docs/flashing.md keeps its non-technical, screenshot-driven audience and tone.

## Todos

- [x] README.md: fix stale pre-release note, tighten Quickstart step 4, general density pass
- [x] COMPATIBILITY.md: reshape preamble narrative into scannable status + verification-semantics sections; keep matrix and all footnotes' facts (matrix verified byte-identical, 43/43 footnote keys balanced)
- [x] docs/runtime.md: density/structure pass — added At-a-glance overview, merged the two storage sections, clustered serial/flags/hardware-IO sections, converted env-var prose to tables; corrected five stale two-board-era claims (arm64-only, RTC, eMMC board list, baud counts) against internal/boards
- [x] docs/provisioning-formats.md: reshape research dossier into explanation+reference (keep commit-pinned citations); corrected stale 5-tier precedence to the implemented 3-tier chain (gosd.toml > cloud-init > config.json), verified against internal/provision
- [x] docs/custom-kernels.md + docs/sound.md: density pass, dedupe overlap (build-kernel mechanics now live only in custom-kernels.md); corrected stale "no board has booted" claims and sound.md's pre-bring-up ROCK 4SE verification table (now points at COMPATIBILITY.md's audio footnotes)
- [x] docs/externals.md + docs/artifacts.md + docs/board-build-tags.md: density pass; artifacts.md was stale (pi-3b and rock-4se missing from every board list, three-way verification procedure absent despite CLAUDE.md citing it as living there) — corrected against build-artifacts.yml and cmd/gosd/build.go
- [x] docs/publishing.md + docs/flashing.md: density pass (flashing stays jargon-free)
- [x] Verify all cross-doc links and anchors still resolve; check code/help-text references to docs paths (scripted check: all md links/anchors across 12 files resolve; Go code references doc paths without anchors, none renamed; examples/chime's deep link into sound.md verified)
- [x] Quality gates + PR (go test / go vet / gofmt / golangci-lint darwin+linux all clean; darwin lint needed the documented stale-worktree cache clean)


## Summary of Changes

A Diátaxis-guided reshape and density pass over README.md, COMPATIBILITY.md,
and all of docs/ (docs/design/ excluded as bean-shaped spike material). No
files renamed (six doc paths are referenced from Go code); all cross-doc
links and the externally-referenced sound.md anchor preserved.

Beyond prose tightening, the pass surfaced and fixed real staleness:

- README.md claimed no GoSD image had ever booted on hardware and named
  artifacts/v0.1.0; replaced with rot-resistant phrasing pointing at
  COMPATIBILITY.md, plus a scannable "Going further" section.
- docs/provisioning-formats.md documented a 5-tier parser precedence from
  before implementation; the shipped parser is 3-tier (gosd.toml >
  cloud-init > config.json) — corrected against internal/provision, and the
  research-dossier framing (section 0, open-questions journal) reshaped into
  settled explanation+reference. Commit-pinned rpi-imager citations kept.
- docs/artifacts.md omitted pi-3b and rock-4se from every board list and
  lacked the three-way verification procedure CLAUDE.md cites as living
  there; both fixed against build-artifacts.yml and cmd/gosd/build.go, and
  the release procedure is now one numbered sequence including the
  tag-first-bump-second rule.
- docs/runtime.md carried five stale two-board-era claims (arm64-only,
  "neither supported board has an RTC", eMMC board list missing ROCK 4SE,
  "two Pi boards" baud phrasing); all corrected against internal/boards.
  Gained an "At a glance" contract summary, merged storage sections, and
  env-var tables.
- docs/custom-kernels.md still said "no board has [booted]" twice;
  docs/sound.md's verification table predated the ROCK 4SE's
  heard-on-hardware milestone (gosd-cfkd) — both now defer to
  COMPATIBILITY.md rather than restating status that drifts.
- COMPATIBILITY.md's dated bring-up narrative became a six-row status table
  plus a crisp statement of the code-complete-vs-hardware-verified legend
  semantics; feature matrix verified byte-identical, all 43 footnote keys
  balanced, every footnote fact retained.


Rebase note: main moved during the pass (#145-#147) and GitHub silently
skips pull_request CI when a PR conflicts (no test-merge commit), which
looked like CI never triggering. Rebased onto main, folding its new
runtime.md sections (Making a write durable, How big the data partition
can be) and footnote cross-references into the reshaped structure. The
rebase also surfaced one more pre-existing contradiction, fixed here:
COMPATIBILITY.md's [^audio] still said no board had been heard on
hardware and called both Rockchip recipes uncompiled, contradicting
[^rock4se-audio]'s heard-on-2026-07-30 record and the matrix's ✅.
