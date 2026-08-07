---
# gosd-zzdz
title: go-diskfs fat32 ReadDir fails on a directory the Linux kernel has written into
status: completed
type: bug
priority: low
created_at: 2026-07-30T22:21:18Z
updated_at: 2026-08-07T17:41:20Z
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

- [x] Reproduce minimally — done as a byte-exact **synthetic** reproduction rather than a live qemu boot capture (see "Why synthetic, not a live boot" below): confirmed the deleted-LFN/SFN shape does **not** fail against the currently pinned go-diskfs v1.9.3
- [x] Decide the fix: **no fix needed.** This entry shape parses correctly today; see "Root-cause finding" below for what "invalid argument" actually is
- [x] Regression coverage added: `TestRootFileExistsSurvivesKernelDeletedEntries` in `internal/diskfmt/diskfmt_test.go` builds the byte-exact shape from the dump above (via a real go-diskfs-written directory, then hand-patched to mark the old entries' first byte `0xE5`, mirroring exactly what a kernel unlink does) and asserts `RootFileExists` still finds the live file and correctly reports the deleted one as absent

## Root-cause finding (2026-08-07)

The "prime suspect" theory in the original report does not hold up under direct testing. Two independent lines of evidence:

1. **Byte-exact synthetic reproduction parses cleanly.** Built a FAT32 image, wrote `hello-boots.tmp` then `hello-boots` side by side via go-diskfs (giving byte-correct LFN+SFN groups: 2 LFN slots + 1 SFN for the `.tmp` name, 1 LFN slot + 1 SFN for the final name — exactly the 5-slot shape in the dump), then overwrote the first byte of the three old-file slots with `0xE5` — the FAT convention, applied to LFN continuation slots exactly as the dump shows. Re-opening the image fresh and calling `fs.ReadDir(".")` returns `["hello-boots"]` with no error. Reading `internal/diskfmt/fat12/directoryentry.go`'s `parseDirEntries` confirms why: the `0xE5` delete check (`switch b[i+0] { case 0xe5: continue }`) runs *before* the code ever looks at the LFN attribute byte (`b[i+11] == 0x0f`), so a deleted LFN slot is skipped exactly like a deleted SFN slot — never treated as a malformed chain. go-diskfs's own `Rename`/`writeDirectoryEntries` always rewrites a directory's entries fresh from its in-memory list, so it can never produce this exact "old entries left in place, marked deleted" shape itself — which is why go-diskfs's own round-trip tests never exercise this path, but it doesn't mean the *read* side mishandles it.

2. **The literal string "invalid argument" is `io/fs.ErrInvalid`,** returned unconditionally by go-diskfs's `validatePath` (`iofs.ValidPath` fails for any rooted path) whenever a caller passes `"/"` instead of `"."` — regardless of directory content, fresh or booted. Confirmed empirically: `fs.ReadDir("/")` fails with exactly `invalid argument` on a brand-new, never-booted, empty FAT32 image too. `internal/diskfmt.go`'s `RootFileExists` already carries a comment about this exact gotcha ("`.` rather than `/`: go-diskfs validates directory paths as io/fs paths, which are unrooted, and rejects a leading slash") — predating this investigation. The simplest explanation consistent with all the evidence: the original host-side inspection script called `ReadDir("/")` literally (with the leading slash), which fails identically on *any* image; it was only ever tried against the booted partition, so the boot was blamed for a failure that has nothing to do with directory content.

Also checked upstream: [go-diskfs#417](https://github.com/diskfs/go-diskfs/issues/417) ("FAT: reading/copying a zero-length file fails — `getClusterList` rejects `firstCluster == 0`", filed 2026-07-25, closed 2026-08-03) is a real, adjacent go-diskfs defect — a zero-cluster/zero-size file (which is what `hello-boots` briefly was mid-investigation, per gosd-0nk4's "before the durability fix" dump: `cluster=0 size=0`) fails with `invalid start cluster: 0` when something reads *that file's own contents* (`OpenFile`/`ReadFile`/`sync.Copy`/`GetClusterChain`). It does **not** fire on a plain `ReadDir` of the *containing* directory, which never resolves each entry's cluster chain (verified by reading `fat12.go`'s `ReadDir`/`readDirWithMkdir` — for `p == "."` it returns straight after parsing the root's own entries). Not filed by this project, no fix is available yet even in the latest tagged go-diskfs (`v1.9.4`, released 2026-07-19, predates both the issue and its fix), and it doesn't match this bean's exact symptom — recorded here only because it's the closest real upstream bug and is relevant if gosd-0nk4-style zero-length files ever need reading back by a host tool.

### Why synthetic, not a live boot

The first todo asked for a live qemu-virt boot capture. This session had `qemu-system-aarch64` and the exact cached artifacts (`v0.8.0`) needed, so a live boot was technically possible, but `scripts/boot-and-grep.sh` (the documented pattern for this) ends with a broad `pkill -f qemu-system-aarch64` — unsafe on this run's shared, multi-agent machine, where that could kill a sibling agent's qemu instance. Building a safer single-PID-tracked variant was judged not worth it once the synthetic byte-exact reproduction (built from the actual captured hex dump in this bean, not a guess) settled the question directly: it reproduces the shape precisely and shows it parses correctly. If this resurfaces on real hardware, a live capture (killing only a PID this run itself started, never `pkill -f`) would be the next step — but per the finding above, the more likely culprit for any future "invalid argument" report is a stray `ReadDir("/")` call, not directory content.

## For gosd-e721 (send-upstream-patches bean)

**No upstream patch is warranted from this investigation.** go-diskfs v1.9.3 already handles the deleted-LFN/SFN-entry shape correctly; there is no bug to patch. The one real, adjacent upstream defect found during this research — [go-diskfs#417](https://github.com/diskfs/go-diskfs/issues/417), zero-length files (`firstCluster == 0`) rejected by `getClusterList` with `invalid start cluster: 0` — was already filed and fixed upstream by someone else (filed 2026-07-25, closed 2026-08-03); no tagged release carries the fix yet (latest tag `v1.9.4` predates the issue). Worth a bump-and-verify pass once go-diskfs cuts a release containing it, if GoSD ever reads back a zero-length file through go-diskfs (e.g. a host tool inspecting a data partition captured mid-write, before gosd-0nk4's four-step durable-write pattern lands its rename). No action needed from gosd-e721 today.

## Summary of Changes

- `internal/diskfmt/diskfmt_test.go`: added `TestRootFileExistsSurvivesKernelDeletedEntries`, which builds the byte-exact "kernel deleted these directory slots" shape from this bean's captured hex dump (via `deleteLFNGroup`, a helper that locates a real go-diskfs-written LFN+SFN group and flips its slots' first byte to `0xE5`) and asserts `RootFileExists` still works correctly against it — pinning today's correct behaviour so a future go-diskfs bump can't silently regress it.
- `internal/diskfmt/diskfmt.go`: added a short note on `RootFileExists` pointing at the new test and stating plainly that this shape is handled correctly, so the next person investigating an "invalid argument" report starts from the `ReadDir("." )` vs `ReadDir("/")` gotcha rather than re-suspecting deleted directory entries.

No workaround/wrapper was added because none reproduces a defect to work around — see "Root-cause finding" above.
