---
# gosd-10fn
title: Rockchip-fleet kernels carry CONFIG_BTRFS_FS=y — defconfig leakage, decide keep or trim
status: todo
type: task
priority: low
created_at: 2026-08-07T09:58:20Z
updated_at: 2026-08-07T09:58:20Z
---

Found 2026-08-07 while grepping recorded kernel.configs for the ext4 epic (gosd-lfu0): radxa-zero-3e, nanopi-zero2, rock-4se and qemu-virt all build btrfs INTO the kernel (arm64 defconfig inheritance — the audit-what-a-defconfig-hands-you trap, Rockchip edition). Nothing in gosd formats or mounts btrfs; it is dead weight in every image and qemu-virt's ForbiddenY does not catch it. Decide: trim it fleet-wide (fragment + ForbiddenY entry + artifacts dance at the next natural release) or keep it deliberately (record why). Low priority; fold the fragment change into the next fleet kernel rebuild rather than cutting a release for it.
