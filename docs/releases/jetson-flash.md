# jetson-flash releases

Release notes for `jetson-flash`, the tool that flashes an image onto a Jetson
Nano eMMC module over USB recovery. Releases are tagged `jetson-flash/vX.Y.Z`
and carry prebuilt Linux binaries.

These are versioned separately from the `gosd` CLI, because the tool changes on
its own schedule and has different users.

This file is maintained by knope from the change files in `.changeset/`; new
versions are added below this heading.
## 0.0.1 (2026-11-10)

### Features

#### First release of `jetson-flash`

`jetson-flash` flashes an OS image onto a Jetson Nano 16GB eMMC module in USB recovery mode, from Linux, with no NVIDIA tools needed.

Each release carries prebuilt binaries for Linux on ARMv7 (which suits the Turing Pi 2's BMC), arm64 and amd64, plus a `jetson-flash-SHA256SUMS` file.
