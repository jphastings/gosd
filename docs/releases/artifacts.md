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
