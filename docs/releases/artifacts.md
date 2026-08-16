# Artifact releases

Release notes for the board artifacts GoSD builds and publishes: the kernels
and U-Boot builds compiled from the pinned sources in `build/boards/*`, tagged
`artifacts/vX.Y.Z` and attached to a GitHub release.

These are versioned separately from the `gosd` CLI, because a board's kernel or
bootloader can change without the CLI changing and vice versa. A CLI release
pins which artifact release it downloads (`internal/artifacts.Version`), and
that pin is bumped in a follow-up PR after the artifacts release exists — see
the artifacts documentation for the full tag-first, bump-second procedure.

This file is maintained by knope from the change files in `.changeset/`; new
versions are added below this heading.
## 0.10.1 (2026-08-16)

### Fixes

#### Cubie A5E images now boot the 1GB RAM variant

The Radxa Cubie A5E's U-Boot now uses DRAM calibration values verified on the
1GB LPDDR4x variant of the board, fixing a U-Boot SPL DRAM-init failure that
previously stopped this variant from booting at all. The 2GB/4GB variants
are not yet hardware-verified and may still have problems; feedback from
anyone running one is welcome (see bean `gosd-84b8`).
