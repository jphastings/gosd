---
artifacts: patch
---

#### The Jetson Nano kernel finds the eMMC module's eMMC

The Jetson Nano's device tree now describes the 16GB eMMC module rather than the SD-card developer kit: the eMMC controller is enabled and always appears as `/dev/mmcblk0`, so gosd-init can mount its boot partition from NVIDIA's `APP` partition. The developer kit's SD slot and M.2 Key E SDIO controllers, which the eMMC module doesn't have, are switched off. The kernel also keeps NVMe support built in, for the drive `/data` will live on.
