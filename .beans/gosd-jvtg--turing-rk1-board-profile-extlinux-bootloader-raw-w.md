---
# gosd-jvtg
title: 'Turing RK1: board profile (extlinux + bootloader raw-writes)'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:26:11Z
updated_at: 2026-08-25T12:29:48Z
parent: gosd-bntd
blocked_by:
    - gosd-k4w2
---

internal/boards/turingrk1: RawWrites (idbloader.img + u-boot.itb at the offsets the research bean confirms), BootFiles (kernel + DTB + initramfs + extlinux.conf), Arch()=arm64, EXT4Support/ConsoleBaudSupport/UsbGadgetSupport per what research found. RegisterInternal in internal/boardset (public activation is a later, separate bean per CLAUDE.md's board-activation pattern). Mirror rock4se's/radxazero3e's board package shape and doc-comment style.



## Progress (2026-08-25)

Board profile, kernelspec entry, U-Boot pipeline, and kernel fragment all
written on branch `bean/gosd-jvtg-turing-rk1-board-profile` (worktree
`../gosd-turing-rk1`). RawWrites offsets confirmed correct by a real build:
`idbloader.img` (202752 bytes) @ LBA64, `u-boot.itb` (1358336 bytes) @
LBA16384 — comfortably inside the 16MiB boot-partition-start ceiling, no
collision. `go build ./...`, `go vet ./...`, `gofmt -l .`, and
`golangci-lint run` (both GOOS) all clean. `go test ./...` clean except the
kernelspec kernel.config-snapshot checks, pending the real
`gosd build-kernel --board turing-rk1` run (in progress in the background —
see gosd-phjh). Will commit + open PR once that lands and the snapshot is
committed.



## Summary of Changes

Board profile (internal/boards/turingrk1), kernelspec entry, kernel
fragment, and full U-Boot Docker pipeline landed together in one PR/commit,
since internal/repocheck mechanically requires a registered board to have
both a kernelspec entry and a build/boards/<id>/ directory in the same
change (registration alone fails CI) -- this merges the scope originally
split across gosd-jvtg/gosd-phjh/gosd-bib8. RawWrites offsets, EXT4Support,
and every RequiredY kernel-fragment assertion were verified against a real
`gosd build-kernel --board turing-rk1` run and a real U-Boot Docker build,
not just research. turing-rk1 registered via RegisterInternal (public
activation is gosd-wf58, after an artifacts release).
