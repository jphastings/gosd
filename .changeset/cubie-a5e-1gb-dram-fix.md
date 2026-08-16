---
artifacts: patch
---

#### Cubie A5E images now boot the 1GB RAM variant

The Radxa Cubie A5E's U-Boot now uses DRAM calibration values verified on the
1GB LPDDR4x variant of the board, fixing a U-Boot SPL DRAM-init failure that
previously stopped this variant from booting at all. The 2GB/4GB variants
are not yet hardware-verified and may still have problems; feedback from
anyone running one is welcome (see bean `gosd-84b8`).
