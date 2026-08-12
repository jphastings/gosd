---
# gosd-g4kf
title: Support (and prove) colima as a container runtime for gosd build-kernel
status: completed
type: task
priority: normal
created_at: 2026-07-11T19:54:40Z
updated_at: 2026-07-11T20:09:20Z
---

JP runs colima (https://colima.run/) locally and asked for explicit support (2026-07-11). Investigation: colima's default docker-runtime mode exposes a standard docker context, so `internal/container.Detect`'s docker path already drives it — in fact every real build in epic [[gosd-47rm]]'s verification (the qemu-virt e2e and the DVB pi-zero-2w proof) ran through colima without us realising: `docker context show` on this machine is `colima`. The daemon-down error already suggests `colima start`.

Consequently this bean is mostly about making support explicit and PROVEN:

- The [[gosd-0p21]] field bug actually occurred under colima's VM (colima shares $HOME and /tmp/colima by default — not /var/folders), not Docker Desktop as the bean/comments state. The fix (mount from under $HOME) is correct for both providers; correct the wording in the code comments and add an addendum to that bean.
- `NotInstalledError` guidance names Docker Desktop and podman but not colima; add it.
- `docs/custom-kernels.md`'s prerequisites/supported-hosts name Docker Desktop and Podman; add colima (docker-runtime mode; nerdctl/containerd mode is out of scope — no docker CLI socket).
- Run the gated real-daemon smoke test (`GOSD_CONTAINER_SMOKE_TEST=1 go test ./internal/container/ ...`) against the running colima and record the result here.

## Summary of Changes

Colima support confirmed and made explicit. Evidence: this machine's docker
context IS colima, so every real build in the epic's verification already ran
through it; additionally `GOSD_CONTAINER_SMOKE_TEST=1 go test
./internal/container/ -run Smoke` passes against the live colima (Detect +
digest-pinned image run, ~2s). Changes: NotInstalledError install guidance
names colima; docs/custom-kernels.md prerequisites + supported-hosts cover
colima (docker-runtime mode only; containerd/nerdctl mode has no docker
socket); kernelbuild mount comment and statedir docstring no longer attribute
VM file-sharing behaviour solely to Docker Desktop; [[gosd-0p21]] gained an
addendum correcting the provider naming (the field bug occurred under colima).
No Detect/Run code changes were needed — colima's docker mode is driven by the
existing docker path by design.

## Todos

- [x] NotInstalledError mentions colima as an install option
- [x] docs/custom-kernels.md prerequisites + supported hosts mention colima (docker mode)
- [x] Correct Docker-Desktop-specific wording in kernelbuild/statedir comments + gosd-0p21 addendum
- [x] Real-colima smoke test run and recorded here
- [x] Quality gates
