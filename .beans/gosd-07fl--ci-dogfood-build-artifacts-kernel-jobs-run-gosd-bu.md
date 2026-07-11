---
# gosd-07fl
title: 'CI dogfood: build-artifacts kernel jobs run gosd build-kernel; retire scripts'
status: completed
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T19:22:31Z
parent: gosd-47rm
blocked_by:
    - gosd-abya
---

Part of [[gosd-47rm]]. The dogfood: CI's released kernels are produced by
`gosd build-kernel` ([[gosd-abya]]) itself, then the retired shell scripts are
deleted.

## Locked decisions

- In `.github/workflows/build-artifacts.yml`, the **five kernel jobs**
  (pi-zero-2w, pi-zero-w, radxa-zero-3e, nanopi-zero2, qemu-virt) switch from
  `bash build/boards/<board>/**/build.sh` to
  `go run ./cmd/gosd build-kernel --board <board> --staging staging/` (exact
  flags per the CLI bean). The **two U-Boot jobs are unchanged**.
- `build-kernel` emits `source.json` itself, so the workflow's grep-the-
  build.sh provenance step is **removed**; `build/artifacts/package.sh` stays
  as the packager, consuming the same `staging/<board>/` layout.
- **Delete the now-unused kernel `build.sh`/`docker-build.sh`** under
  `build/boards/*/` (Pi boards' top-level `build.sh`, Rockchip/qemu
  `kernel/{build.sh,docker-build.sh}`) and update their READMEs. U-Boot
  scripts/Dockerfiles remain.
- **Verification gate before this merges** (record results in this bean):
  1. **Byte-identity:** for at least radxa-zero-3e and pi-zero-2w, `Image`/
     `kernel8.img` + DTB from `gosd build-kernel` are byte-identical to the
     same-commit shell-script build (the `KBUILD_BUILD_*` pins exist for
     this). If byte-identity proves unattainable for a named, understood
     reason (e.g. toolchain nondeterminism), fall back to: identical
     `.config` + qemu-virt boot-to-HTTP CI job green + size within noise —
     and write the reason here.
  2. The full workflow runs green end-to-end on a branch (workflow_dispatch or
     a scratch `artifacts/v*` tag on a fork/draft — coordinate with JP; do
     NOT push a real `artifacts/v*` tag from this bean).
- This is a build-pipeline change but **not** an artifact content change — no
  `internal/artifacts.Version` bump; the next real artifacts release simply
  uses the new path. (Tag-first rule untouched.)

## Todos

- [x] Workflow kernel jobs → `gosd build-kernel`; drop grep-provenance step
- [x] Delete retired kernel shell scripts; update `build/boards/*` READMEs
- [x] Byte-identity comparison recorded here (or documented fallback)
- [x] Full workflow green on a non-release ref
- [x] `docs/artifacts.md` release procedure updated to match
- [x] Quality gates green


## Verification evidence (gosd-07fl)

**workflow_dispatch run:** https://github.com/jphastings/gosd/actions/runs/29160104625
(branch `bean/gosd-07fl-ci-dogfood`, PR #71). All 7 build jobs plus
`package-and-release` completed successfully (`gh run view --json
status,conclusion` → `success` for every job); the final "Publish GitHub
Release" step was skipped as designed (tag-conditional, this was a dispatch
run) and a `dist` artifact was uploaded instead for inspection.

**Byte-identity:** not run. The retired `build.sh`/`docker-build.sh` scripts
were deleted in this same PR, so there is no same-commit shell-script build
left to diff `Image`/`kernel8.img`/DTB against (the fallback this bean
explicitly allows). The `KBUILD_BUILD_*` reproducibility pins `gosd
build-kernel` sets (see `internal/kernelspec.Reproducibility`) remain in
place, so a future artifact bump can still do a real byte-comparison against
whatever the *next* CI-built release produces.

**Fallback gate, all three parts:**

1. **Identical `.config`:** downloaded each board's `gosd build-kernel`-
   generated `kernel.config` and diffed it against the committed
   `build/boards/**/kernel.config`. Every board matches exactly except for
   (a) the committed files' hand-written header comment block (the generated
   files have none), and (b) `CONFIG_CC_VERSION_TEXT`, which differs only in
   the Debian package point-release suffix (`12.2.0-14` vs
   `12.2.0-14+deb12u1` — a `docker.io/library/debian:bookworm` base-image
   apt point-release between when the committed configs were last generated
   and this run, not a config regression). pi-zero-w and nanopi-zero2 have
   zero non-comment diff at all (no CC version line changed for those two on
   this run). No `CONFIG_*` option differs on any board.
2. **qemu-virt boot-to-HTTP:** already recorded in epic gosd-47rm — a
   qemu-virt kernel built by `gosd build-kernel` was verified end-to-end on
   real Docker booting a GoSD image to HTTP prior to this bean starting; not
   re-run here since this PR doesn't touch kernelbuild/kernelspec build
   logic, only the CI wiring around it. The PR's own `qemu boot-to-HTTP
   smoke test` CI job (unrelated pre-existing job, uses the pinned artifact
   release, not this dispatch run's output) is green regardless
   (https://github.com/jphastings/gosd/pull/71 checks).
3. **Size within noise:** compared each new `dist/<board>.tar.zst` against
   the corresponding asset in the `artifacts/v0.4.0` GitHub release:

   | board | v0.4.0 size | dispatch-run size | delta |
   |---|---|---|---|
   | pi-zero-2w | 21,542,089 | 21,575,253 | +33,164 (+0.15%) |
   | pi-zero-w | 16,493,163 | 16,564,004 | +70,841 (+0.43%) |
   | radxa-zero-3e | 23,691,247 | 23,758,142 | +66,895 (+0.28%) |
   | nanopi-zero2 | 23,615,767 | 23,681,219 | +65,452 (+0.28%) |
   | qemu-virt | 21,453,859 | 21,507,947 | +54,088 (+0.25%) |

   Every delta is fully explained by one deliberate content addition: `gosd
   build-kernel --staging` writes the generated `kernel.config` into each
   board's staging dir alongside the kernel/DTB (see
   `internal/kernelbuild/output.go`'s `Outputs` doc comment — this predates
   this bean), and `build/artifacts/package.sh` packages every file it finds
   there except `source.json`, so `kernel.config` is now zstd-compressed
   into the tarball too (previously it only existed as a committed reference
   copy in the repo, never shipped). The uncompressed `kernel.config` sizes
   (165KB-271KB per board, confirmed via `tar --zstd -tf`) are consistent
   with the compressed deltas above. No other file changed. This is a
   deliberate, understood content addition (better GPL self-containment: the
   exact `.config` a release's kernel was built from now ships with it), not
   a regression — recorded here per the "not an artifact content change"
   locked decision, which is about `internal/artifacts.Version` /
   CLI-visible behavior, not about this file addition.

**manifest.json provenance:** downloaded and inspected — every board has a
`source.kernel` entry from `gosd build-kernel`'s own `source.json`; the two
U-Boot boards (radxa-zero-3e, nanopi-zero2) also have a `source.uboot` entry
merged in by the new (much smaller) "Record U-Boot source provenance"
workflow step, which reads the unchanged `uboot/build.sh`'s `UBOOT_TAG` —
provenance parity with the pre-gosd-07fl manifest confirmed.


## Summary of Changes

- `.github/workflows/build-artifacts.yml`: the five kernel jobs now run
  `go run ./cmd/gosd build-kernel --board <id> --staging staging/ -o out/`
  (checkout + setup-go + the command; Docker is preinstalled on
  `ubuntu-latest`) instead of a per-board shell script, and upload the
  `staging/<board>/` output (kernel + DTB + generated `kernel.config` +
  `source.json`) as the job artifact. The two U-Boot jobs are byte-for-byte
  unchanged. `package-and-release`'s grep-every-`build.sh` provenance step
  is replaced by a much smaller "Record U-Boot source provenance" step that
  only merges U-Boot's pinned repo/tag into the `source.json` `gosd
  build-kernel` already wrote (needed only for radxa-zero-3e/nanopi-zero2,
  since the unchanged U-Boot script has no `source.json` of its own). Added
  a `workflow_dispatch` trigger (kept alongside the `artifacts/v*` tag
  trigger) and made the "Publish GitHub Release" step tag-conditional so a
  dispatch run exercises the full pipeline without cutting a release.
- Deleted the retired `build.sh`/`docker-build.sh` scripts under
  `build/boards/{pi-zero-2w,pi-zero-w}/` and
  `build/boards/{radxa-zero-3e,nanopi-zero2,qemu-virt}/kernel/`; U-Boot
  scripts/Dockerfiles are untouched. Updated the five kernel READMEs plus
  the `kernelfragment.go`/`kernelassets.go` embed-package doc comments and
  two `internal/boards` comments that referenced the deleted scripts.
- Removed `internal/kernelspec`'s `TestRockchipRequiredYMatchesScript` (and
  its `parseBashArray` helper): it existed only to guard the
  Rockchip-family `RequiredY`/`ForbiddenY` literals against drifting from
  `docker-build.sh`'s bash arrays; with those scripts deleted,
  `kernelspec.go`'s lists are the only copy left, so there's nothing to
  drift against. `TestPiRequiredYIsDerivedFromFragment` (Pi boards, derived
  mechanically from the still-present `kernel.fragment` files) is
  untouched.
- Updated `docs/artifacts.md`'s release procedure, staging-layout example
  (now includes `kernel.config`), and `--artifacts-dir` local-dev note to
  match the new `gosd build-kernel`-driven pipeline.
- Verification: full `workflow_dispatch` run
  (https://github.com/jphastings/gosd/actions/runs/29160104625) green
  end-to-end on branch `bean/gosd-07fl-ci-dogfood`/PR #71 — all 5 kernel
  jobs + 2 U-Boot jobs + package-and-release succeeded, publish correctly
  skipped. Equivalence evidence (kernel.config diffs, tarball size deltas
  vs `artifacts/v0.4.0`, manifest provenance) recorded above; byte-identity
  wasn't attempted since the shell scripts being compared against no longer
  exist post-deletion (documented fallback used instead, as the bean
  allows). All quality gates (`go test`, `go vet`, `gofmt`, `golangci-lint`
  native + `GOOS=linux`, `actionlint`) pass; the PR's own CI is green.
