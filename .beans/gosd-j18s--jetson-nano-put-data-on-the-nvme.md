---
# gosd-j18s
title: 'Jetson Nano: put /data on the NVMe'
status: todo
type: feature
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-07T20:59:10Z
parent: gosd-72je
---

JP decided on 2026-11-07 that the eMMC module keeps `/data` on an NVMe drive, not on the eMMC. On the bench, that's the 477 GB drive in the Turing Pi 2's node 4 M.2 slot, which NVIDIA's U-Boot already sees as `nvme0`. The reasons:

- NVIDIA's GPT owns the eMMC.
- `APP` (partition 1, 14 GiB) holds GoSD's FAT boot partition.
- Every reflash rewrites the eMMC from scratch (see the flashing guide).

**Today, on this board, nothing breaks but nothing persists:**
- gosd-init only looks for data at `mmcblk{0,1}p2` and `vda2`, which on this board is NVIDIA's `DTB` partition.
- `dataexpand` only understands GoSD's MBR, and refuses NVIDIA's GPT before it writes anything.

So the app runs with an empty, read-only `/data`.

**Design:**
- How a board declares "data lives on NVMe".
- How gosd-init finds, creates and adopts the data partition on `nvme0n1`. Reuse the format/grow/adopt-behind-a-completion-marker machinery in `internal/blockmount` that `disk/` and `emmc/` share; don't invent a second path.
- How the label (`<prefix>-data`), `--data-filesystem` and `--data-size` apply.
- What happens with no NVMe fitted. Fail fast with an actionable error, per the fail-fast decision.
- A guarantee that nothing on the eMMC but `APP` is ever written.
- Kernel: check `CONFIG_BLK_DEV_NVME` against the fragment. `PCI_TEGRA` is already required.

**Before review:** per CLAUDE.md, this formats and adopts on-disk state, so it needs an explicit crash-ordering argument and an adversarial review pass before JP's review.
