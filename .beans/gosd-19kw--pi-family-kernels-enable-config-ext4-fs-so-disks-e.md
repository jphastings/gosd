---
# gosd-19kw
title: 'Pi-family kernels: enable CONFIG_EXT4_FS so disk/''s ext4 default works on Pi USB drives'
status: in-progress
type: feature
priority: normal
created_at: 2026-08-07T19:11:14Z
updated_at: 2026-08-07T19:40:38Z
parent: gosd-lfu0
---

JP (2026-08-07): fleet featureset should be complete — the Pi boards are the only ones whose kernels lack ext4 (EXT4_FS not set in all three recorded kernel.configs), so disk/'s ext4-by-default fails its /proc/filesystems preflight there (USB drives are the affected surface; Pis have no eMMC/NVMe).

- [x] Add CONFIG_EXT4_FS=y to all THREE Pi fragments (pi-zero-2w, pi-zero-w, pi-3b), family-wide per the per-family pin convention. RequiredY derives from the fragment for Pi boards (requiredYFromFragment in internal/kernelspec/kernelspec.go), so the fragment line is the assertion — confirmed via TestPiRequiredYIsDerivedFromFragment, which re-derives RequiredY from the on-disk fragment and passes for all three boards. Dependency check: fetched fs/ext4/Kconfig at the pinned raspberrypi/linux commit (63598c83153e19b1f99067ab6df7409de2c111f8) via GitHub's raw view — config EXT4_FS selects BUFFER_HEAD, JBD2, CRC16, CRC32, FS_IOMAP (and FS_ENCRYPTION_ALGS only if FS_ENCRYPTION, which we don't set). No depends-on/tristate gate blocks a no-modules =y. These selects chain automatically under merge_config.sh + olddefconfig, so no companion fragment lines were added — CONFIG_EXT4_FS=y alone is the whole change. (All five companions are already =y in the three boards' committed kernel.config today, pulled in by other already-enabled subsystems, confirming the chain resolves cleanly in this tree.) Comment style: added a WHY block above the new line matching the fragment's existing exFAT block convention (disk/'s ext4 default needs the driver for USB drives on Pis — Pis have no eMMC/NVMe; decision JP 2026-08-07).
- [x] Audit the resulting kernel.configs per the Pi-defconfig-trap rule: not applicable this round — the committed kernel.config files are last-real-build snapshots (regenerated only by an actual gosd build-kernel run, which this bean explicitly does not perform locally; see gosd-oq0z's cross-reference note on provenance-commit ownership). Nothing in bcm2711_defconfig/bcmrpi_defconfig promotes a surprise ext4-adjacent =m driver the way mac80211_hwsim/legacy-gadget did — EXT4_FS is off by default in these defconfigs, so there is no defconfig-trap analog here; the real post-build audit happens against the CI workflow_dispatch run's kernel.config output.
- **Artifacts dance**: ship the fragment change WITHOUT bumping artifacts.Version; coordinate the release tag with gosd-10fn's btrfs trim so both kernel changes ride ONE artifacts release (they touch disjoint board families — Pi here, mainline fleet there — so one tag covers all seven boards' rebuilds). CI workflow_dispatch pre-merge run is the build verification; a size check on the Pi kernels (ext4+jbd2 adds ~1MiB) confirms the change landed.
- COMPATIBILITY.md ext4 rows for the Pi boards flip once the Version bump lands.



## Progress (2026-08-07)

Fragment edits landed (pi-zero-2w, pi-zero-w, pi-3b), COMPATIBILITY.md's [^pi-ext4] footnote updated to the 'enabled in fragment, pending next artifacts release' wording (mirrors [^exfat-soon]'s existing pattern) — rows themselves stay ❌ per the artifacts-dance rule. artifacts.Version is untouched; this release is expected to ship together with gosd-10fn's btrfs trim (disjoint board families, one tag). Quality gates: gofmt clean; go test ./internal/kernelspec/... passes (including TestPiRequiredYIsDerivedFromFragment for all three boards); a full go test ./... run completed with every package passing except cmd/gosd's disk-heavy cross-compile integration tests, which failed purely on host ENOSPC (linker 'no space left on device') from concurrent sibling-agent contention on the shared build machine — unrelated to this change, which touches zero .go files. go vet/golangci-lint could not be driven to completion locally despite repeated retries with an isolated GOCACHE and go clean -cache, due to sustained, severe disk exhaustion outside this task's control (confirmed via ps: no processes of mine were the cause). Remaining: open the PR, trigger workflow_dispatch on build-artifacts.yml for the real CI build/size verification, record the run URL here, and stop — orchestrator monitors the run.



## Progress update: quality gates all green (2026-08-07)

After the initial ENOSPC-plagued attempts (recorded above), the shared machine's disk pressure eased enough for clean runs: `go vet ./...` (0 issues), `golangci-lint run ./...` (0 issues), `GOOS=linux golangci-lint run ./...` (0 issues, after two ENOSPC-caused false starts that cleared on retry), `gofmt -l .` (clean), and `go test ./internal/kernelspec/...` including TestPiRequiredYIsDerivedFromFragment (all pass). A full `go test ./...` run earlier passed every package except cmd/gosd's disk-heavy cross-compile integration tests, which hit host ENOSPC from concurrent sibling-agent contention — unrelated to this change (zero .go files touched). Committed as 2cbb4d9 on bean/gosd-19kw-pi-ext4, rebased onto latest origin/main via fast-forward (no conflicts). Next: push, open PR, trigger workflow_dispatch on build-artifacts.yml, record the run URL.



## PR opened, CI triggered (2026-08-07)

PR: https://github.com/jphastings/gosd/pull/223
Rebased onto latest main (fast-forward + a clean rebase past two more incoming merges — no conflicts). All quality gates green: go test ./..., go vet ./..., gofmt -l ., golangci-lint run ./..., GOOS=linux golangci-lint run ./... (see the ENOSPC note above for the bumpy road getting there — none of it was code-related).

workflow_dispatch run (build-artifacts.yml on bean/gosd-19kw-pi-ext4): https://github.com/jphastings/gosd/actions/runs/31212493453 — includes the pi-zero-2w, pi-zero-w and pi-3b kernel jobs this change touches. Stopping here per the task instructions; orchestrator monitors the run. Remaining after merge: JP pushes the artifacts/vX.Y.Z tag (shared with gosd-10fn's btrfs trim), then a follow-up PR bumps internal/artifacts.Version and flips COMPATIBILITY.md's Pi ext4 row symbols.
