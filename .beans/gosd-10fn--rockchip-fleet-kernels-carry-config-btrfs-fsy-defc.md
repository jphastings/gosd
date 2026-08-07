---
# gosd-10fn
title: Rockchip-fleet kernels carry CONFIG_BTRFS_FS=y — defconfig leakage, decide keep or trim
status: in-progress
type: task
priority: normal
created_at: 2026-08-07T09:58:20Z
updated_at: 2026-08-07T20:52:59Z
---

Found 2026-08-07 while grepping recorded kernel.configs for the ext4 epic (gosd-lfu0): radxa-zero-3e, nanopi-zero2, rock-4se and qemu-virt all build btrfs INTO the kernel (arm64 defconfig inheritance — the audit-what-a-defconfig-hands-you trap, Rockchip edition). Nothing in gosd formats or mounts btrfs; it is dead weight in every image and qemu-virt's ForbiddenY does not catch it. Decide: trim it fleet-wide (fragment + ForbiddenY entry + artifacts dance at the next natural release) or keep it deliberately (record why). Low priority; fold the fragment change into the next fleet kernel rebuild rather than cutting a release for it.

## Decision (JP, 2026-08-07): TRIM btrfs

Remove CONFIG_BTRFS_FS from every mainline-fleet kernel (explicit disable in each fragment: radxa-zero-3e, nanopi-zero2, rock-4se, cubie-a5e; qemu-virt gains a ForbiddenY entry — nothing in gosd formats or mounts it). MEDIA_SUPPORT's keep-or-trim remains UNDECIDED — this bean's remaining open question; do not trim it on the same pass without JP's explicit call. Ship the fragment change in the SAME artifacts release window as the Pi ext4 enablement (see that bean) — one tag, seven rebuilt boards, one three-way verification.



## Progress

Landed the TRIM decision's fragment + ForbiddenY half:

- Added an explicit `# CONFIG_BTRFS_FS is not set` to the four
  mainline-fleet fragments (radxa-zero-3e, nanopi-zero2, rock-4se,
  cubie-a5e), each with a one-line WHY comment mirroring the surrounding
  "Cut" block style (defconfig leakage promoted to =y by the no-modules
  build, nothing in gosd formats or mounts btrfs, JP 2026-08-07).
- Added `CONFIG_BTRFS_FS` to qemu-virt's ForbiddenY in
  internal/kernelspec/kernelspec.go, with a comment mirroring the
  existing CONFIG_USB_MASS_STORAGE entry's style, so drift is caught
  fleet-wide.
- Checked the three Pi boards' recorded kernel.config
  (build/boards/{pi-3b,pi-zero-2w,pi-zero-w}/kernel.config): all three
  already carry `# CONFIG_BTRFS_FS is not set` explicitly. Confirmed
  absent as expected — no Pi changes made.
- MEDIA_SUPPORT untouched, as directed.
- Did NOT bump internal/artifacts.Version — this rides with gosd-19kw's
  Pi ext4 enablement in the same release window (one tag, one three-way
  verification), per the locked decision above.
- Triggered `gh workflow run build-artifacts.yml --ref bean/gosd-10fn-btrfs-trim`
  after opening the PR; run URL recorded below.

Quality gates: go vet ./..., gofmt -l ., golangci-lint run ./... and
GOOS=linux golangci-lint run ./... all clean. go test ./... passes for
every package including internal/kernelspec (the only Go package this
bean touches); cmd/gosd's own integration-test suite could not be gotten
green in this session — every failure across roughly 20 foreground
retries was literally "no space left on device" from the shared
/var/folders temp volume (other sibling agents/worktrees on this
machine), never an assertion tied to CONFIG_BTRFS_FS or the fragment
files. Deferring to CI (a clean runner) to confirm cmd/gosd green.

Status stays in-progress: this bean completes when the artifacts release
carrying the trim actually ships.



## CI dispatch run

Triggered `gh workflow run build-artifacts.yml --ref bean/gosd-10fn-btrfs-trim`:
https://github.com/jphastings/gosd/actions/runs/31212440098

PR: https://github.com/jphastings/gosd/pull/222

Dispatch run 31212440098 FAILED exactly as the new ForbiddenY assertion should: qemu-virt's built config still carried BTRFS_FS=y — the fragment disable had only gone to the four hardware boards (the bean's own wording framed qemu-virt as assertion-only, which was wrong since it shares the defconfig baseline). Added the explicit disable to qemu-virt's fragment too; re-dispatched.

Reconciliation: a parallel session independently recorded the same JP decision on main (commit dad9d21) with 'ride the next natural fleet rebuild, no dedicated release' framing and its own todo list; this bean's record supersedes it — the 'next natural release' is the one already in flight (shared artifacts tag with gosd-19kw's Pi ext4, PRs #222/#223), and the fragment+ForbiddenY work those todos described is what PR #222 contains.
