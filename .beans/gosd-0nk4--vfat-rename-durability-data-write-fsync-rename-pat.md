---
# gosd-0nk4
title: 'vfat rename durability: /data write-fsync-rename pattern isn''t durable until ~30s writeback expiry'
status: completed
type: bug
priority: normal
created_at: 2026-07-30T20:42:07Z
updated_at: 2026-08-21T07:39:57Z
---

Found during gosd-6sac's qemu boot-cycle testing (2026-07-30), by killing qemu at varying delays after boot: examples/hello's boot counter never survived a <30s-after-write power cut, across many runs. Host-side mount of the card showed the tell: `hello-boots.tmp` (2 bytes, the new counter, fsync'd) present on disk, `hello-boots` absent — the rename's directory update never reached the card.

Mechanism: GOSD-DATA is mounted vfat with the `flush` option, which flushes a file's data+inode on close(2) — that's why the .tmp file and gosd-init's .gosd-data marker persist almost immediately. But os.Rename involves no close: the directory mutation just dirties pages that wait for the normal writeback expiry (dirty_expire_centisecs, default 30s). A power cut in that window loses the rename entirely.

Strictly, docs/runtime.md's promise holds — readers see the OLD version (no file) or the new, never a torn mix — but the implied 'fsync makes it durable' expectation is broken: there is a ~30s window where a completed write-fsync-rename vanishes. Real bench sessions run long past the window, which is why hardware bring-ups never caught it.

## Kernel-source verdict: vfat directories DO support fsync — and it alone is NOT enough

Checked against both pinned trees: mainline `v6.18.37` (`fleetKernelTag`, Rockchip boards + qemu-virt) and raspberrypi/linux `63598c83` (`piZeroCommitRef`, the Pi boards). The relevant fs/fat code is byte-identical between them, at identical line numbers.

**Directory fsync works, and does more than you'd expect:**

- `fs/fat/dir.c:878` — `fat_dir_operations` wires `.fsync = fat_file_fsync`. Directories are not the "no fsync method" case; `fsync(2)` on a directory fd is a real operation on vfat.
- `fs/fat/file.c:186-200` — `fat_file_fsync()` = `__generic_file_fsync()` + `sync_mapping_buffers(sbi->fat_inode->i_mapping)` (the FAT table itself) + `blkdev_issue_flush(sb->s_bdev)` (a device cache-flush command).
- `fs/libfs.c:1524-1553` — `__generic_file_fsync()` calls `sync_mapping_buffers(inode->i_mapping)`, which writes **and waits for** the buffers hanging off that inode's private buffer list.
- `fs/fat/dir.c:1027,1062,1114,1195,1257,1358,1369` — every directory-entry mutation (`fat_add_entries`, `fat_remove_entries`, `fat_alloc_new_dir`) dirties its buffer with `mark_buffer_dirty_inode(bh, dir)`, i.e. onto exactly that private list of the **directory** inode.

So `fsync` on the directory fd writes the added and removed directory entries, waits for them, syncs the FAT table, and issues a card-cache flush. That's the cheap correct fix the bean hoped for.

**But it is not sufficient on its own** — this is the trap, and it is worse than the bug it fixes:

- `fs/fat/namei_vfat.c:933` — `vfat_rename()` creates the new entry with `vfat_add_entry(new_dir, name, is_dir, 0 /* cluster */, …)`: **start cluster 0, size 0**.
- `fs/fat/namei_vfat.c:904` — it then calls `vfat_sync_ipos()`, which on a non-`dirsync` mount only does `mark_inode_dirty(inode)`. The real start cluster and size reach that new entry only when the inode is later written back.
- `fs/fat/inode.c:853` — `__fat_write_inode()` stamps size/start into the entry and marks the buffer dirty with plain `mark_buffer_dirty(bh)` (the *blockdev's* list, NOT the directory's private list), writing it synchronously only when called with `wait=1` — which is what a **file** `fsync` does via `sync_inode_metadata(inode, 1)`.

Consequence: fsync **only** the directory after a rename and a power cut can persist the new name as a **0-byte file** — the name flips, the contents are gone. Verified experimentally, not just read off the source (see evidence below).

For completeness on why `flush` doesn't cover any of this: `fat_flush_inodes()` (`fs/fat/inode.c:1888`) is called from `fat_file_release()` (`fs/fat/file.c:175`) — i.e. on `close(2)`, which a rename never performs.

## Implemented

The documented durable-write sequence is now four steps, and `examples/hello`'s `writeFileDurably` (plus `examples/emmcstorage`'s copy of it) does all four:

1. write to `<name>.tmp`, `fsync` it — contents on the card;
2. `rename` over the real name — atomic flip for readers;
3. `fsync` the renamed file, through the still-open handle — writes its new directory entry with the real start cluster and size;
4. `fsync` the directory — writes the entry the rename added and the one it removed, syncs the FAT, flushes the card's cache.

`docs/runtime.md` gains a "Making a write durable" subsection saying exactly what steps 1-2 guarantee (crash *consistency*: old or new, never torn), what steps 3-4 add (*durability* against an immediate power cut), why step 3 can't be skipped, that the cost is a few extra small card writes, and that stopping at steps 1-2 is a legitimate choice for data you'd be happy to lose. The eMMC/disk sections and COMPATIBILITY.md's FAT footnote point at it; `docs/design/ab-updates.md`'s commit protocol (§4) notes it too, since its "renamed and durable" rows silently assumed durability the rename doesn't provide.

## Evidence (local qemu, arm64 host, real qemu-virt image)

`gosd build ./examples/hello --board=qemu-virt --data-size=64MiB`, booted with `scripts/qemu-run.sh`, SIGKILLing qemu a fixed delay after hello's startup line (which prints *after* the counter write). Killing qemu is a faithful power cut: everything the guest actually submitted is in the host's page cache, everything it didn't is lost.

| Variant | Kill delay | Result |
|---|---|---|
| Before (fsync + rename) | 3s | boots=1, boots=1, boots=1 — **bug reproduced**, counter never advances |
| Before | 40s alive, then kill | next boot reads boots=2 — confirms the ~30s writeback expiry is the whole story |
| Directory fsync only | 3s | boots=1, boots=1 — name persisted, **contents didn't**: the on-card entry is `HELLO-~1` with cluster 0, size 0 |
| After (file fsync + dir fsync) | 2s | boots=1 → 2 → 3 → 4 |
| After | 0s (immediately at the log line) | boots=5 → 6 → 7 |

Raw directory dump of the data partition (host-side, no mount — the FAT32 root at image offset 0x11100000), before vs after:

```
before:  LFN 'hello-boots.t' / 'mp'  +  SFN 'HELLO-~1TMP' cluster=16384 size=2   ← only the .tmp
         (no hello-boots entry at all)

dir-only: SFN 'åELLO-~1TMP' (0xE5 deleted)  +  LFN 'hello-boots' + SFN 'HELLO-~1' cluster=0 size=0
after:    SFN 'åELLO-~1TMP' (0xE5 deleted)  +  LFN 'hello-boots' + SFN 'HELLO-~1' cluster=16384 size=2
```

Regression cover: CI's existing `qemu-expand-data` job already boots the same image twice, killing qemu seconds after the first boot's counter write, so the second boot's assertions now include `boots=2` — no new job, no extra boot. Verified locally both ways: passes on this branch, and fails with `serial log is missing: boots=2` when `examples/hello` is reverted to the old two-step write.

## Decided: no `dirsync` (JP, 2026-07-31) — analysis kept for the record

`dirsync` would make correctness the default for every app — no four-step dance to learn — by making every directory mutation synchronous. On vfat it closes both halves of the trap: `IS_DIRSYNC(dir)` makes `fat_add_entries`/`fat_remove_entries` call `sync_dirty_buffer()` on each modified entry buffer, and `vfat_sync_ipos()` call `fat_sync_inode()`, so `rename(2)` returns with the entry fully on the card, cluster and size included.

What it would cost:

- **Every** directory-modifying syscall becomes a synchronous small write plus a wait — and one write-temp-then-rename cycle performs several (create the temp entry, add the new entry, remove the old one, stamp the inode). On SD/eMMC a small synchronous write is a read-modify-write of a much larger erase block inside the card: single-digit milliseconds on a good card, 100ms+ stalls on a cheap one. An app that persists state in a loop gets measurably slower, and its flash wear goes up for the same bytes of data.
- It's fleet-wide and applies to apps that never needed it, including ones deliberately writing scratch files fast.
- It does **not** remove the need for `fsync` on the data itself — an app still has to fsync file contents before the rename — so it removes the surprise, not the discipline.
- It cannot be opted out of per-app once mounted (short of new gosd.toml machinery).

Where it would change, if you want it:

- `cmd/gosd-init/internal/boot/mounts.go:207` — the `/data` mount's `"flush"` option string, plus the assertion in `cmd/gosd-init/internal/boot/mounts_test.go:217`. This is the narrow version: `/data` only.
- `internal/blockmount/platform_linux.go:21` — `vfatMountOptions = "flush"`, shared by `emmc` and `disk`. Changing this one is the fleet-wide version and hits every mounted volume, including big media drives where a directory-heavy workload is most likely.
- `docs/runtime.md` — the durability section would flip from "your app must do steps 3-4" to "the mount does it for you", and the examples would shed `syncDir`.

**Recommendation: don't.** The four-step pattern is documented, demonstrated in two examples, and costs nothing for apps that don't call it; `dirsync` taxes every app on every card write to protect the minority who write durable state, on hardware where small synchronous writes are the expensive operation. If the calculus ever changes (say a lot of third-party apps get this wrong), the narrow `/data`-only change above is a one-word diff and is the version to take.

## Todos

- [x] Establish from the pinned kernel sources whether vfat implements fsync on a directory fd (both trees, with line cites)
- [x] Implement the durable sequence in `examples/hello` (and `examples/emmcstorage`, which carried the same helper)
- [x] Make `docs/runtime.md` honest about what the pattern does and doesn't guarantee, and what to do for an immediate-power-cut-proof write
- [x] Reproduce the loss in qemu and demonstrate the fix by kill-at-varying-delays
- [x] Prove the "directory fsync alone" trap experimentally rather than only from the source
- [x] Lock it in with a CI assertion, verified to fail against the pre-fix code
- [x] File the go-diskfs `ReadDir` issue separately: **gosd-zzdz**
- [x] JP: decide on the `dirsync` mount option — **DECIDED 2026-07-31: no.**
      JP: "let's avoid dirsync for now - let apps make their own choice."
      Locked: GoSD mounts `/data` without `dirsync`, and durability is the
      app's decision via the documented four-step pattern. The reasoning, if
      this is ever revisited: `dirsync` would tax every card write to protect
      the minority of apps that write durable state, on media where small
      synchronous writes are the expensive operation — and it still would not
      remove the need to fsync file *data*, so it buys a permanent cost
      without removing the sharp edge. The narrow `/data`-only change is a
      one-word diff (`mounts.go`) if the calculus ever changes.
- [x] Optional, not needed for confidence: repeat one kill-cycle on real hardware via sdwire (qemu ran our real kernel and mount options, so this only adds card-level realism) — **decided 2026-08-21: not doing it.** Judged unnecessary, not forgotten; see the Summary of Changes for what the qemu evidence already covers and what a card would have added.


## Summary of Changes

Closed 2026-08-21. Everything this bean set out to establish is on `main` and
CI-guarded; the one remaining todo was explicitly optional and is deliberately
not being done.

**What was proven, and by what means.** The loss was reproduced and the fix
demonstrated on a real `qemu-virt` image — gosd's own kernel, gosd's own
`/data` mount options, gosd's own `examples/hello` — by SIGKILLing qemu at
fixed delays after the counter write. SIGKILL is a faithful power cut at the
guest boundary: everything the guest submitted is in the host's page cache,
everything it didn't is gone. The table above is that run: three boots at
`boots=1` before the fix, `boots=1 → 2 → 3 → 4` after it, and — the finding
that mattered most — a directory-fsync-only variant persisting the NAME with
cluster 0 and size 0, which is worse than the bug it appeared to fix. The
kernel-source verdict was read from both pinned trees with line cites, and
then confirmed experimentally rather than trusted.

**What shipped.** The four-step durable-write sequence in `examples/hello` and
`examples/emmcstorage`, `docs/runtime.md`'s "Making a write durable" section
(honest about what steps 1-2 guarantee versus what 3-4 add, and why step 3
can't be skipped), and a regression assertion folded into CI's existing
double-boot qemu jobs — verified to fail against the pre-fix example, which is
the part that makes it a real gate rather than a passing test.

**The optional hardware repeat: judged unnecessary, not forgotten.** An sdwire
kill-cycle on a real card would add exactly one variable the qemu runs don't
have — the card's own controller, its erase-block behaviour and its volatile
write cache. It would not touch the mechanism this bean is about, which lives
entirely in `fs/fat`: which buffers a rename dirties, whose private list they
hang off, and which fsync waits for them. That mechanism is decided above the
block device, identically for a virtio disk and an SD card, and
`fat_file_fsync`'s `blkdev_issue_flush` is precisely the call that pushes the
question past a card's cache. A real card could therefore only fail this in a
way that would equally break every other durable write GoSD documents, and
CI re-runs the qemu proof on every push while a bench repeat would be a
one-off. The evidence is sufficient; the extra realism isn't worth a bench
slot.

**The `dirsync` decision stands** (JP, 2026-07-31): `/data` is mounted without
it, durability is the app's choice through the documented four-step pattern,
and the one-word change site is recorded above if the calculus ever changes.
That decision is also recorded as a project-wide locked decision in CLAUDE.md,
so it survives this bean's closure.
