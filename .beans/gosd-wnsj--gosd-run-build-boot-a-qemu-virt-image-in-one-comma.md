---
# gosd-wnsj
title: 'gosd run: build + boot a qemu-virt image in one command'
status: todo
type: feature
created_at: 2026-07-05T15:24:35Z
updated_at: 2026-07-05T15:24:35Z
---

Follow-up to gosd-27lz (scripts/qemu-run.sh). Wrap the build-extract-boot flow in a first-class 'gosd run <pkg>' command: cross-compile, assemble a qemu-virt image, extract Image/initramfs from the FAT partition (internal/cmd/imgextract logic, callable directly rather than via go run), and exec qemu-system-aarch64 with serial on stdio and hostfwd 8080->80. scripts/qemu-run.sh proved the invocation; this promotes it from a repo-local dev script to something app developers get from the installed CLI. Needs a decision on qemu-binary discovery/version floor and on flag surface (port mapping, memory, extra qemu args).
