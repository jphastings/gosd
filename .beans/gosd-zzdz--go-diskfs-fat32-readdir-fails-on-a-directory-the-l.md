---
# gosd-zzdz
title: go-diskfs fat32 ReadDir fails on a directory the Linux kernel has written into
status: todo
type: bug
priority: low
created_at: 2026-07-30T22:21:18Z
updated_at: 2026-07-30T22:21:18Z
---

go-diskfs v1.9.3's FAT32 reader refuses to list a directory the Linux kernel has written into: `ReadDir("/")` on the GOSD-DATA partition of an image that has been booted comes back `invalid argument`, while the same call on a freshly-`gosd build`-created partition works. Host-side inspection tooling only — devices are unaffected, since the kernel's own vfat driver reads those directories fine.

Noticed while investigating gosd-0nk4 (2026-07-30): after a boot in which `examples/hello` did its write-temp/rename dance, a host-side `ReadDir("/")` over the data partition fails. Prime suspect is the deleted-entry markers the rename leaves behind — an LFN entry whose sequence byte has been overwritten with `0xE5` (deleted), which a strict reader may treat as a malformed long-name chain rather than "skip me". A raw dump of the same directory after a boot+kill shows exactly that shape:

```
0x11100060 LFN seq=e5 'mp'
0x11100080 LFN seq=e5 'hello-boots.t'
0x111000a0 SFN 'åELLO-~1TMP' attr=20   (0xE5 = deleted)
0x111000c0 LFN seq=41 'hello-boots'
0x111000e0 SFN 'HELLO-~1   ' attr=20
```

Why it matters: `internal/diskfmt`/`internal/blockmount` and the build-integration tests read images back through go-diskfs, so any future test or tool that inspects a *booted* card's data partition (rather than a freshly built one) will hit this. It also means "mount the image on the host and look" is not available as a debugging technique for the data partition on macOS without a real vfat mount.

## Todos

- [ ] Reproduce minimally: build a qemu-virt image with a data partition, boot it once under qemu (`scripts/qemu-run.sh`), then `ReadDir("/")` the data partition with go-diskfs; confirm the error and pin down which entry shape triggers it
- [ ] Decide the fix: upstream patch/issue against go-diskfs, a local tolerant-reader wrapper in `internal/diskfmt`, or "document and avoid"
- [ ] If a workaround lands in-tree, cover it with a fixture directory containing deleted LFN/SFN entries so a go-diskfs bump can't silently regress it
