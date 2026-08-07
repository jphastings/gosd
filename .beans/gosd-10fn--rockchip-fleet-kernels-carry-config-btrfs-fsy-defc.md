---
# gosd-10fn
title: Rockchip-fleet kernels carry CONFIG_BTRFS_FS=y — defconfig leakage, decide keep or trim
status: todo
type: task
priority: normal
created_at: 2026-08-07T09:58:20Z
updated_at: 2026-08-07T20:31:14Z
---

Found 2026-08-07 while grepping recorded kernel.configs for the ext4 epic (gosd-lfu0): radxa-zero-3e, nanopi-zero2, rock-4se and qemu-virt all build btrfs INTO the kernel (arm64 defconfig inheritance — the audit-what-a-defconfig-hands-you trap, Rockchip edition). Nothing in gosd formats or mounts btrfs; it is dead weight in every image and qemu-virt's ForbiddenY does not catch it. Decide: trim it fleet-wide (fragment + ForbiddenY entry + artifacts dance at the next natural release) or keep it deliberately (record why). Low priority; fold the fragment change into the next fleet kernel rebuild rather than cutting a release for it.

## Decision (JP, 2026-08-07): TRIM btrfs

Remove CONFIG_BTRFS_FS from every mainline-fleet kernel (explicit disable in each fragment: radxa-zero-3e, nanopi-zero2, rock-4se, cubie-a5e; qemu-virt gains a ForbiddenY entry — nothing in gosd formats or mounts it). MEDIA_SUPPORT's keep-or-trim remains UNDECIDED — this bean's remaining open question; do not trim it on the same pass without JP's explicit call. Ship the fragment change in the SAME artifacts release window as the Pi ext4 enablement (see that bean) — one tag, seven rebuilt boards, one three-way verification.



---

**Decided (JP, 2026-08-07): TRIM at the next natural fleet kernel rebuild** — no dedicated artifacts release. Concretely: drop CONFIG_BTRFS_FS via the fleet fragment, add a ForbiddenY entry for qemu-virt (and the fleet boards' list if applicable) so it cannot leak back in, and ride the next fleet kernel bump's release dance. Todos:

[ ] Fleet fragment: disable CONFIG_BTRFS_FS explicitly
[ ] kernelspec ForbiddenY entry so the leak is caught by tests
[ ] Ride the next fleet kernel tag bump (do NOT cut a release for this)
