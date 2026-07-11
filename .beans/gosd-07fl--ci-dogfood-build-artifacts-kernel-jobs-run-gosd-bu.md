---
# gosd-07fl
title: 'CI dogfood: build-artifacts kernel jobs run gosd build-kernel; retire scripts'
status: in-progress
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T16:50:23Z
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
- [ ] Byte-identity comparison recorded here (or documented fallback)
- [ ] Full workflow green on a non-release ref
- [x] `docs/artifacts.md` release procedure updated to match
- [x] Quality gates green
