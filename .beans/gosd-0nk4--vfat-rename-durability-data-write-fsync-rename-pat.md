---
# gosd-0nk4
title: 'vfat rename durability: /data write-fsync-rename pattern isn''t durable until ~30s writeback expiry'
status: todo
type: bug
created_at: 2026-07-30T20:42:07Z
updated_at: 2026-07-30T20:42:07Z
---

Found during gosd-6sac's qemu boot-cycle testing (2026-07-30), by killing qemu at varying delays after boot: examples/hello's boot counter never survived a <30s-after-write power cut, across many runs. Host-side mount of the card showed the tell: `hello-boots.tmp` (2 bytes, the new counter, fsync'd) present on disk, `hello-boots` absent — the rename's directory update never reached the card.

Mechanism: GOSD-DATA is mounted vfat with the `flush` option, which flushes a file's data+inode on close(2) — that's why the .tmp file and gosd-init's .gosd-data marker persist almost immediately. But os.Rename involves no close: the directory mutation just dirties pages that wait for the normal writeback expiry (dirty_expire_centisecs, default 30s). A power cut in that window loses the rename entirely.

Strictly, docs/runtime.md's promise holds — readers see the OLD version (no file) or the new, never a torn mix — but the implied 'fsync makes it durable' expectation is broken: there is a ~30s window where a completed write-fsync-rename vanishes. Real bench sessions run long past the window, which is why hardware bring-ups never caught it.

Fix directions to evaluate (pick in the bean when picked up):
- Test whether Linux vfat supports fsync on a directory fd (fat_file_fsync is wired for files; check directories). If yes: document 'open the dir, Sync it after rename' in runtime.md and do it in examples/hello.
- syscall.Syncfs on /data after rename (heavier, but always works). unix.Syncfs exists on Linux.
- Mount option change (e.g. adding `dirsync`) — makes every directory op synchronous on the card: correctness by default, at a real write-amplification/latency cost on SD. Consider for the data mount specifically.

Also noticed nearby (separate, minor): go-diskfs v1.9.3's fat32 reader fails ReadDir('/') with 'invalid argument' on a directory the Linux kernel has written into (deleted-entry markers from the rename dance are the suspect) — only affects host-side inspection tooling, not devices.
