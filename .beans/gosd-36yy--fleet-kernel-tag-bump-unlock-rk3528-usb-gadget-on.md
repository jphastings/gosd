---
# gosd-36yy
title: 'Fleet kernel tag bump: unlock RK3528 USB gadget on NanoPi Zero2'
status: todo
type: task
priority: deferred
created_at: 2026-07-24T16:02:21Z
updated_at: 2026-07-24T16:17:22Z
parent: gosd-cwjf
---

Fleet-wide kernel tag bump (v6.18.37 -> TBD) so the RK3528 dwc3/USB DT node
lands on NanoPi Zero2, unlocking USB gadget mode there — the single most
wanted unlock right now (goal: full usbwebsite flow = gadget + eMMC on one
board; nanopi-zero2 is that board post-bump).

## Provenance / bean-ID note

The task brief that spawned this work named bean **gosd-vcae** as "the task
bean" and instructed `beans update gosd-vcae -s in-progress`. gosd-vcae is
NanoPi Zero2's mainline-viability research bean — it is **status: completed**
(closed months ago, its own PR merged) and its scope was "does mainline
support exist for this board at all", not "bump the fleet kernel tag". Per
CLAUDE.md ("if a locked decision proves wrong in practice, stop and say so
in the bean rather than silently diverging") this bean (gosd-vcae) is not
reopened/hijacked for unrelated new work — that would corrupt its closed
history and violate "one bean = one branch = one PR". Instead this new bean
carries the fleet-bump work; it is filed as a child of gosd-cwjf (the NanoPi
Zero2 board-support epic), which already carries the exact breadcrumb this
bean acts on:

> ## USB gate discovered during kernel build (2026-07-06, PR #33)
> rk3528.dtsi has NO USB controller node in any numbered kernel release as of
> v6.18.37 — the RK3528 dwc3 node is merged on Linus master only... Recheck
> when bumping KERNEL_TAG past the release containing the rk3528 dwc3 node.

gosd-vcae and gosd-rqx8 (NanoPi Zero2 kernel build, still in-progress —
its one open todo is hardware boot-test, tracked separately) are the
evidentiary basis for "why this bump is needed"; this bean is the bump
itself.

## Locked decisions (per CLAUDE.md "Board work & artifact releases")

- All boards pin the SAME fleet kernel tag — bump together, never one board
  alone.
- Ship the build/kernelspec change WITHOUT bumping internal/artifacts.Version
  in this PR (tag-first-bump-second rule) — JP tags the artifacts release,
  a follow-up bean bumps Version and does the 3-way verify.
- Do not hand-edit any kernel.config (generated at build).
- Do not flip COMPATIBILITY.md gadget-row statuses (post-release + hardware
  verify only) — footnote text naming the old tag may be updated.

## Todos

[x] Choose target tag: newest stable/LTS point release with the RK3528
    dwc3 controller DT node in rk3528.dtsi, verified against real sources —
    OUTCOME: no such release exists yet (see Summary). No tag chosen; bump
    deferred.
[x] Verify every board's DTS patches (rock-4se, radxa-zero-3e, nanopi-zero2)
    still apply at the new tag; rebase any that don't — done as forward-prep
    against v7.2-rc4 (the only ref with the node) even though it isn't the
    chosen target; all 7 patches across 3 boards apply clean. Not applied to
    the tree since no bump occurs in this PR.
[x] NanoPi Zero2: promote the fragment's inert USB symbols to required per
    its own comment block; verify Kconfig symbols still exist at new tag;
    decide on a dr_mode=peripheral DTS patch (rock-4se 0003 is the pattern)
    — decision recorded in Summary (recommend the same peripheral-override
    pattern as rock-4se); not applied to the tree since no bump occurs.
[ ] Update fleetKernelTag + all stale "v6.18.37" references repo-wide — NOT
    DONE: no eligible release tag exists to move to (see Summary). Tag stays
    v6.18.37; COMPATIBILITY.md's [^nanopi-usb] footnote enhanced with the
    new evidence instead (still accurate at the unchanged pinned tag).
[ ] Update kernelspec_test.go board-enumerating assertions as needed — N/A,
    no kernelspec change in this PR.
[x] Full quality gates green (go test/vet/gofmt/golangci-lint x2) — run
    against the bean/docs-only diff.
[ ] Push branch, `gh workflow run build-artifacts.yml --ref <branch>`,
    record the run URL here — SKIPPED deliberately: no build-affecting
    change in this PR (fleetKernelTag unchanged), so a dispatch run would
    just rebuild main unchanged and add no signal. Re-run this step in the
    eventual bump PR instead.
[x] PR opened; bean status/priority set to reflect "blocked on upstream,
    revisit later" rather than active in-progress work (see status/priority)

## Summary of Changes

**Outcome: no tag bump in this PR.** The RK3528 dwc3/USB DT node this bean
exists to bring in does not appear in any tagged, numbered kernel release
yet — only on an active mainline release-candidate for the *next major
version*. Per this bean's own locked decision rule ("if the node only exists
outside LTS, STOP, record the tradeoff, flag for JP"), this is exactly that
case, and more starkly than anticipated. Recorded below with full evidence;
nothing under `build/boards/*` or `internal/kernelspec` changes in this PR.
Only this bean and `COMPATIBILITY.md`'s `[^nanopi-usb]` footnote (evidence
refresh, status left ❌ per the locked no-flip rule) change.

### 1. Current pin is still the right LTS line

`fleetKernelTag = "v6.18.37"`. Cross-checked against
`https://www.kernel.org/releases.json` (fetched 2026-07-24): the 6.18.x line
is `"moniker": "longterm"`, `"iseol": false` — still an actively maintained
LTS line (latest point release `v6.18.40`, released 2026-07-24). No reason
to move off the 6.18 LTS lineage on maintenance grounds; this bean is only
about the USB node.

### 2. Where the RK3528 USB node actually lives — checked directly, tag by tag

`arch/arm64/boot/dts/rockchip/rk3528.dtsi`'s USB/dwc3 controller node
(`usb_host0_xhci`, `compatible = "rockchip,rk3528-dwc3","snps,dwc3"`, plus
`usb_host0_ehci`/`usb_host0_ohci`/`usb2phy*`) was checked by fetching the
real file at each ref (`raw.githubusercontent.com/torvalds/linux/<tag>/...`
for tags that exist there; `git.kernel.org/.../stable/linux.git/plain/...
?h=<tag>` via `curl`/`git ls-remote` for point releases — the mirror's plain
endpoint is Anubis-bot-protected against the WebFetch tool specifically, so
this was done with direct `curl`/`git` in Bash instead, which was not
blocked):

| Ref | Has USB/dwc3 node? |
|---|---|
| v6.18 | No (only QoS placeholder nodes + disabled naneng-combphy) |
| v6.18.40 (latest 6.18 LTS point, today) | No |
| v6.19 / v6.19.14 (latest point) | No |
| v7.0 / v7.0.14 (latest point, EOL) | No |
| v7.1 / v7.1.5 (latest point, current "stable" per releases.json) | No |
| v7.2-rc4 (current mainline pre-release, tagged 2026-07-19) | **Yes** |

`master` (Linus's tree HEAD) matches v7.2-rc4's content for this file.
`releases.json`'s mainline entry is `"version": "7.2-rc4"` today, confirming
v7.2 has not shipped as a numbered release.

### 3. The exact commits, for the record

- `5f3ae9b12a6c` "arm64: dts: rockchip: Add USB nodes for RK3528" —
  2026-06-02, https://github.com/torvalds/linux/commit/5f3ae9b12a6c — adds
  the SoC-level `usb_host0_xhci`/`ehci`/`ohci`/`usb2phy*` nodes to
  `rk3528.dtsi`.
- `ff660109f412` "arm64: dts: rockchip: Enable USB 2.0 ports on NanoPi
  Zero2" — 2026-06-02, https://github.com/torvalds/linux/commit/ff660109f412
  — board-level enablement. Commit message: *"The NanoPi Zero2 has one USB
  2.0 Type-A HOST port and one USB 2.0 Type-C OTG port."* This is the exact
  commit gosd-vcae's and gosd-cwjf's original research anticipated, now
  identified precisely.

Both commits post-date v7.1 and are only reachable from `master`/v7.2-rc4 —
not backported to any active stable or LTS branch. This is expected, not a
bug in anyone's process: new-hardware DT-node additions are feature
additions, not bug fixes, and are essentially never accepted into
stable/LTS branches per `Documentation/process/stable-kernel-rules.rst`.
Empirically confirmed here too — even the two *already-superseded* v7.0.x/
v7.1.x lines (chronologically closer to the commits than 6.18/6.19) don't
have them either.

### 4. Why this bean does NOT recommend pinning to v7.2-rc4 now

- **Locked architecture, not a small deviation.** `internal/kernelspec.go`'s
  own doc comment: `fleetKernelTag and fleetKernelRepo pin the same mainline
  stable LTS tag across every Rockchip-family board`. Every Rockchip board
  uses `TagRef` against the *stable* tree specifically. Pinning a `-rc` tag
  from the *unstable* dev branch would be a first-of-its-kind precedent
  change for this codebase (the Pi boards' `CommitRef` against a vendor's
  own long-lived branch is a different, not-comparable case) — a decision
  only JP should make, not something to slide in as a side effect of this
  bean.
- **It's a major-version jump, not a point bump.** v7.2 is a new major
  version relative to the pinned 6.18 line. "Bump the fleet tag" as scoped
  by this bean assumed a same-lineage point-release move; moving to 7.2
  would need full defconfig/Kconfig-survival re-verification across every
  board, which is a materially bigger project than this bean's remit.
- **rc content isn't final.** `v7.2-rc4` can still change before `v7.2.0`
  ships; building a release pipeline against it means re-verifying again
  anyway once the real tag lands, with the added risk of shipping on
  content that gets rebased/dropped before release.
- **Discovered en route:** `radxa-zero-3e`'s upstream board DTS has already
  been refactored to `#include "rk3566-radxa-zero-3.dtsi"` (a new shared
  base for the Zero 3 family) by v7.2-rc4, moving content that used to live
  directly in the board file (see patch-applicability note below — it still
  applied cleanly today, but this is a sign that boards' upstream files
  don't stay static, another reason a careful, scoped bump beats grabbing
  the first tag with the wanted node).

**Recommendation:** wait for `v7.2.0` (or whichever numbered release first
ships these two commits) to actually be tagged, then re-run this bean's
tag-selection step against that real release. `v7.2-rc4` was tagged
2026-07-19; typical mainline rc cadence (~weekly, usually 7-8 rcs) puts a
rough, non-committal estimate of `v7.2.0` landing in roughly 4-6 weeks
(~late Aug/early Sep 2026). Flagging this whole tradeoff for JP rather than
unilaterally picking either "wait" or "pin to rc" outright, though "wait"
is this bean's recommendation.

### 5. Bonus due diligence: patch applicability at v7.2-rc4 (forward-prep only)

Since the only ref with the node is v7.2-rc4, all 7 existing Rockchip DTS
patches were dry-run against the *real* v7.2-rc4 board DTS files (fetched
live, applied sequentially per board with `patch -p1 --fuzz=0 --dry-run`,
then for real to chain multi-patch boards correctly) as prep work for
whenever the actual bump happens — this is evidence for the future bump,
not something applied to the tree now:

| Board | Patch | Applies clean at v7.2-rc4? |
|---|---|---|
| nanopi-zero2 | 0001-enable-header-i2c5.patch | Yes |
| nanopi-zero2 | 0002-enable-header-spi1.patch | Yes (after 0001) |
| rock-4se | 0001-enable-header-i2c.patch | Yes |
| rock-4se | 0002-enable-header-spi.patch | Yes (after 0001) |
| rock-4se | 0003-usb-dwc3-peripheral.patch | Yes (after 0001+0002) |
| radxa-zero-3e | 0001-enable-header-i2c3.patch | Yes |
| radxa-zero-3e | 0002-enable-header-spi3.patch | Yes (after 0001) |

7/7 apply with zero fuzz today. For radxa-zero-3e specifically, despite the
board DTS now `#include`ing a shared family dtsi and being much shorter
than before, `patch`'s context search still lands the insertion in a
syntactically correct spot (verified by reading the patched output, not
just trusting "no conflict") — but this is a snapshot of today's rc4
content, not a guarantee about the final v7.2.0 diff.

### 6. NanoPi Zero2 USB enablement plan — pre-baked for the eventual bump

- The fragment's own comment block
  (`build/boards/nanopi-zero2/kernel/kernel-fragment.config` lines 70-97)
  already says what to do: promote the currently-inert USB symbols to
  required once the node lands. `CONFIG_USB_DWC3` is confirmed still the
  right symbol name at v7.2-rc4 (used verbatim in
  `drivers/usb/dwc3/Kconfig`'s guard, unchanged). `CONFIG_PHY_ROCKCHIP_INNO_USB2`
  needs a fresh check that `phy-rockchip-inno-usb2.c`'s `of_device_id` table
  gained an `rk3528` entry alongside the new node (not done in this pass —
  left as an explicit to-do for whoever does the real bump, since it wasn't
  worth fully Kconfig-verifying a change this bean isn't making).
- **New DTS patch will be needed, and its shape is now known precisely.**
  Unlike rock-4se (bean gosd-je2r found *zero* OTG/extcon glue on that
  board, forcing `dr_mode = "peripheral"` by elimination), NanoPi Zero2's
  Type-C port is a genuine OTG port — the enablement commit's own message
  says so, and the board DTS override sets `extcon = <&usb2phy>` (SoC
  default `dr_mode = "otg"` at `rk3528.dtsi`'s `usb_host0_xhci`, unchanged
  by the board file). Despite that, this bean still recommends forcing
  `dr_mode = "peripheral"` on `&usb_host0_xhci` (same pattern as rock-4se's
  `0003-usb-dwc3-peripheral.patch`) **because the board already has a
  separate, dedicated USB 2.0 Type-A host port** (`usb_host0_ehci`/
  `usb_host0_ohci`) — dedicating the Type-C DWC3 controller to gadget mode
  loses no host capability on this board and gives GoSD's configfs gadget
  stack a deterministic UDC at boot instead of depending on extcon/ID-pin
  role-switch timing (the same class of problem `g_mass_storage`
  auto-binding caused on rock-4se, bean gosd-z9l4). This is a
  recommendation for the eventual patch author to confirm against GoSD's
  actual gadget-bringup pattern, not something applied here.

### 7. COMPATIBILITY.md

`[^nanopi-usb]` footnote enhanced with the commit-level evidence above (the
exact commits, dates, and "not in any release yet" status) so the next
person doesn't have to re-derive it. The ❌ status is unchanged — per this
bean's locked decision, status flips only happen after a real artifact
release + hardware verification.

### 8. Quality gates / CI dispatch

`go test ./...`, `go vet ./...`, `gofmt -l .`, and both `golangci-lint run
./...` invocations were run against this (docs/bean-only) diff and are
clean — see PR. `gh workflow run build-artifacts.yml` was deliberately
**not** dispatched: nothing under `build/boards/*` or `internal/kernelspec`
changed, so a dispatch run would rebuild identically to `main` and provide
no verification signal for this PR. The eventual real bump PR should do the
dispatch step.

### What's still open

- Recheck trigger: `v7.2.0` (or whatever release first ships commits
  `5f3ae9b12a6c` / `ff660109f412`) getting tagged in the stable tree — or,
  less likely, either commit getting backported to an active LTS/stable
  line.
- When that happens: re-run tag selection, apply the 7 verified-compatible
  patches (re-verify against the *actual* released tag, not today's rc4
  snapshot), add the new nanopi-zero2 USB DTS patch per the plan in §6,
  promote the fragment's USB symbols, update `fleetKernelTag`, update
  `kernelspec_test.go`'s RequiredY-style assertions, run the full quality
  gates, dispatch `build-artifacts.yml`, and only then does the normal
  tag-first/bump-second artifact-release dance in CLAUDE.md apply.
- This bean's priority is set to `deferred` (mirroring gosd-vcae's own
  "if no-go, set priority to deferred with a recheck note" playbook) and
  status to `todo` — it is not actively worked, but is not done either.
